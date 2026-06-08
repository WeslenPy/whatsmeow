// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"go.mau.fi/util/random"

	"go.mau.fi/whatsmeow/socket"
	"go.mau.fi/whatsmeow/util/cbcutil"
)

// UploadResponse contains the data from the attachment upload, which can be put into a message to send the attachment.
type UploadResponse struct {
	URL        string `json:"url"`
	DirectPath string `json:"direct_path"`
	Handle     string `json:"handle"`
	ObjectID   string `json:"object_id"`

	MediaKey      []byte `json:"-"`
	FileEncSHA256 []byte `json:"-"`
	FileSHA256    []byte `json:"-"`
	FileLength    uint64 `json:"-"`

	FBID      string `json:"fbid"`
	MetaHMAC  string `json:"meta_hmac"`
	Timestamp int64  `json:"ts"`

	// RawResponse contem o corpo JSON cru retornado pelo servidor de upload.
	// Util para inspecionar campos que nao estao mapeados na struct (ex.: a
	// resposta de biz-cover-photo com fbid/meta_hmac/ts).
	RawResponse []byte `json:"-"`
}

// Upload uploads the given attachment to WhatsApp servers.
//
// You should copy the fields in the response to the corresponding fields in a protobuf message.
//
// For example, to send an image:
//
//	resp, err := cli.Upload(context.Background(), yourImageBytes, whatsmeow.MediaImage)
//	// handle error
//
//	imageMsg := &waE2E.ImageMessage{
//		Caption:  proto.String("Hello, world!"),
//		Mimetype: proto.String("image/png"), // replace this with the actual mime type
//		// you can also optionally add other fields like ContextInfo and JpegThumbnail here
//
//		URL:           &resp.URL,
//		DirectPath:    &resp.DirectPath,
//		MediaKey:      resp.MediaKey,
//		FileEncSHA256: resp.FileEncSHA256,
//		FileSHA256:    resp.FileSHA256,
//		FileLength:    &resp.FileLength,
//	}
//	_, err = cli.SendMessage(context.Background(), targetJID, &waE2E.Message{
//		ImageMessage: imageMsg,
//	})
//	// handle error again
//
// The same applies to the other message types like DocumentMessage, just replace the struct type and Message field name.
func (cli *Client) Upload(ctx context.Context, plaintext []byte, appInfo MediaType) (resp UploadResponse, err error) {
	resp.FileLength = uint64(len(plaintext))
	resp.MediaKey = random.Bytes(32)

	plaintextSHA256 := sha256.Sum256(plaintext)
	resp.FileSHA256 = plaintextSHA256[:]

	iv, cipherKey, macKey, _ := getMediaKeys(resp.MediaKey, appInfo)

	var ciphertext []byte
	ciphertext, err = cbcutil.Encrypt(cipherKey, iv, plaintext)
	if err != nil {
		err = fmt.Errorf("failed to encrypt file: %w", err)
		return
	}

	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	dataToUpload := append(ciphertext, h.Sum(nil)[:10]...)

	dataHash := sha256.Sum256(dataToUpload)
	resp.FileEncSHA256 = dataHash[:]

	cli.Log.Debugf("Uploading media: type=%s plaintextLen=%d encLen=%d fileSHA256=%s fileEncSHA256=%s",
		appInfo, resp.FileLength, len(dataToUpload),
		base64.StdEncoding.EncodeToString(resp.FileSHA256),
		base64.StdEncoding.EncodeToString(resp.FileEncSHA256))

	err = cli.rawUpload(ctx, bytes.NewReader(dataToUpload), uint64(len(dataToUpload)), resp.FileEncSHA256, appInfo, false, &resp)
	if err != nil {
		cli.Log.Errorf("Media upload failed (type=%s): %v", appInfo, err)
		return
	}
	cli.Log.Debugf("Media upload response: %+v", resp)
	return
}

// UploadReader uploads the given attachment to WhatsApp servers.
//
// This is otherwise identical to [Upload], but it reads the plaintext from an [io.Reader] instead of a byte slice.
// A temporary file is required for the encryption process. If tempFile is nil, a temporary file will be created
// and deleted after the upload.
//
// To use only one file, pass the same file as both plaintext and tempFile. This will cause the file to be overwritten with encrypted data.
func (cli *Client) UploadReader(ctx context.Context, plaintext io.Reader, tempFile io.ReadWriteSeeker, appInfo MediaType) (resp UploadResponse, err error) {
	resp.MediaKey = random.Bytes(32)
	iv, cipherKey, macKey, _ := getMediaKeys(resp.MediaKey, appInfo)
	if tempFile == nil {
		tempFile, err = os.CreateTemp("", "whatsmeow-upload-*")
		if err != nil {
			err = fmt.Errorf("failed to create temporary file: %w", err)
			return
		}
		defer func() {
			tempFileFile := tempFile.(*os.File)
			_ = tempFileFile.Close()
			_ = os.Remove(tempFileFile.Name())
		}()
	}
	var uploadSize uint64
	resp.FileSHA256, resp.FileEncSHA256, resp.FileLength, uploadSize, err = cbcutil.EncryptStream(cipherKey, iv, macKey, plaintext, tempFile)
	if err != nil {
		err = fmt.Errorf("failed to encrypt file: %w", err)
		return
	}
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("failed to seek to start of temporary file: %w", err)
		return
	}
	err = cli.rawUpload(ctx, tempFile, uploadSize, resp.FileEncSHA256, appInfo, false, &resp)
	return
}

// UploadNewsletter uploads the given attachment to WhatsApp servers without encrypting it first.
//
// Newsletter media works mostly the same way as normal media, with a few differences:
// * Since it's unencrypted, there's no MediaKey or FileEncSHA256 fields.
// * There's a "media handle" that needs to be passed in SendRequestExtra.
//
// Example:
//
//	resp, err := cli.UploadNewsletter(context.Background(), yourImageBytes, whatsmeow.MediaImage)
//	// handle error
//
//	imageMsg := &waE2E.ImageMessage{
//		// Caption, mime type and other such fields work like normal
//		Caption:  proto.String("Hello, world!"),
//		Mimetype: proto.String("image/png"),
//
//		// URL and direct path are also there like normal media
//		URL:        &resp.URL,
//		DirectPath: &resp.DirectPath,
//		FileSHA256: resp.FileSHA256,
//		FileLength: &resp.FileLength,
//		// Newsletter media isn't encrypted, so the media key and file enc sha fields are not applicable
//	}
//	_, err = cli.SendMessage(context.Background(), newsletterJID, &waE2E.Message{
//		ImageMessage: imageMsg,
//	}, whatsmeow.SendRequestExtra{
//		// Unlike normal media, newsletters also include a "media handle" in the send request.
//		MediaHandle: resp.Handle,
//	})
//	// handle error again
func (cli *Client) UploadNewsletter(ctx context.Context, data []byte, appInfo MediaType) (resp UploadResponse, err error) {
	resp.FileLength = uint64(len(data))
	hash := sha256.Sum256(data)
	resp.FileSHA256 = hash[:]
	err = cli.rawUpload(ctx, bytes.NewReader(data), resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}

// UploadNewsletterReader uploads the given attachment to WhatsApp servers without encrypting it first.
//
// This is otherwise identical to [UploadNewsletter], but it reads the plaintext from an [io.Reader] instead of a byte slice.
// Unlike [UploadReader], this does not require a temporary file. However, the data needs to be hashed first,
// so an [io.ReadSeeker] is required to be able to read the data twice.
func (cli *Client) UploadNewsletterReader(ctx context.Context, data io.ReadSeeker, appInfo MediaType) (resp UploadResponse, err error) {
	hasher := sha256.New()
	var fileLength int64
	fileLength, err = io.Copy(hasher, data)
	resp.FileLength = uint64(fileLength)
	resp.FileSHA256 = hasher.Sum(nil)
	_, err = data.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("failed to seek to start of data: %w", err)
		return
	}
	err = cli.rawUpload(ctx, data, resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}

// UploadProductCatalogImage uploads an image to be used in a business catalog product.
//
// Unlike [Upload], catalog images are uploaded unencrypted (similar to newsletter media),
// and the resulting URL/DirectPath in the response can be used directly in a product node.
func (cli *Client) UploadProductCatalogImage(ctx context.Context, data []byte) (resp UploadResponse, err error) {
	resp.FileLength = uint64(len(data))
	hash := sha256.Sum256(data)
	resp.FileSHA256 = hash[:]
	err = cli.rawUpload(ctx, bytes.NewReader(data), resp.FileLength, resp.FileSHA256, MediaProductCatalogImage, false, &resp)
	return
}

func (cli *Client) rawUpload(ctx context.Context, dataToUpload io.Reader, uploadSize uint64, fileHash []byte, appInfo MediaType, newsletter bool, resp *UploadResponse) error {
	mediaConn, err := cli.refreshMediaConn(ctx, false)
	if err != nil {
		return fmt.Errorf("failed to refresh media connections: %w", err)
	}

	token := base64.URLEncoding.EncodeToString(fileHash)
	q := url.Values{
		"auth":  []string{mediaConn.Auth},
		"token": []string{token},
	}
	mmsType := mediaTypeToMMSType[appInfo]
	uploadPrefix := "mms"

	// Product catalog images are uploaded unencrypted to the /product/image endpoint.
	if appInfo == MediaProductCatalogImage {
		uploadPrefix = "product"
	}

	if appInfo == MediaBizCoverPhoto {
		appInfo = MediaImage
	}

	if mmsType == "biz-cover-photo" {
		uploadPrefix = "pps"
	}

	if cli.MessengerConfig != nil {
		uploadPrefix = "wa-msgr/mms"
		// Messenger upload only allows voice messages, not audio files
		if mmsType == "audio" {
			mmsType = "ptt"
		}
	}
	if newsletter {
		mmsType = fmt.Sprintf("newsletter-%s", mmsType)
		uploadPrefix = "newsletter"
	}
	var host string
	// Hacky hack to prefer last option (rupload.facebook.com) for messenger uploads.
	// For some reason, the primary host doesn't work, even though it has the <upload/> tag.
	if cli.MessengerConfig != nil {
		host = mediaConn.Hosts[len(mediaConn.Hosts)-1].Hostname
	} else {
		host = mediaConn.Hosts[0].Hostname
	}
	uploadURL := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     fmt.Sprintf("/%s/%s/%s", uploadPrefix, mmsType, token),
		RawQuery: q.Encode(),
	}

	cli.Log.Debugf("Upload URL: %s", uploadURL.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), dataToUpload)
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}

	req.ContentLength = int64(uploadSize)
	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Referer", socket.Origin+"/")

	httpResp, err := cli.mediaHTTP.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to execute request: %w", err)
	} else if httpResp.StatusCode != http.StatusOK {
		// Le o corpo mesmo em erro para logar a mensagem retornada pelo servidor.
		body, _ := io.ReadAll(httpResp.Body)
		resp.RawResponse = body
		cli.Log.Debugf("Upload response (status %d): %s", httpResp.StatusCode, body)
		err = fmt.Errorf("upload failed with status code %d", httpResp.StatusCode)
	} else {
		var body []byte
		if body, err = io.ReadAll(httpResp.Body); err != nil {
			err = fmt.Errorf("failed to read upload response: %w", err)
		} else {
			resp.RawResponse = body
			cli.Log.Debugf("Upload response JSON: %s", body)
			if err = json.Unmarshal(body, &resp); err != nil {
				err = fmt.Errorf("failed to parse upload response: %w", err)
			}
		}
	}
	if httpResp != nil {
		_ = httpResp.Body.Close()
	}
	return err
}

// DeleteMedia deletes the media at the given direct path from WhatsApp servers.
//
// This is only used for things like history syncs, which should be deleted after processing.
func (cli *Client) DeleteMedia(ctx context.Context, appInfo MediaType, directPath string, encFileHash []byte, encHandle string) error {
	mediaConn, err := cli.refreshMediaConn(ctx, false)
	if err != nil {
		return fmt.Errorf("failed to refresh media connections: %w", err)
	}

	queryStart := strings.IndexByte(directPath, '?')
	if queryStart > 0 {
		directPath = directPath[:queryStart]
	}

	token := base64.URLEncoding.EncodeToString(encFileHash)
	query := url.Values{
		"token": []string{token},
		"d_md":  []string{base64.RawURLEncoding.EncodeToString([]byte(directPath))},
		"auth":  []string{mediaConn.Auth},
	}
	if encHandle != "" {
		query.Set("e_handle", encHandle)
	}
	deleteURL := url.URL{
		Scheme:   "https",
		Host:     mediaConn.Hosts[0].Hostname,
		Path:     fmt.Sprintf("/mms/%s/%s", mediaTypeToMMSType[appInfo], token),
		RawQuery: query.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}

	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Referer", socket.Origin+"/")
	// TODO non-on-demand backfills may require this? it's in the initial bootstrap payload and may need to be persisted
	//req.Header.Set("Companion_User_Secret", companionMetaNonce)

	httpResp, err := cli.mediaHTTP.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to execute request: %w", err)
	} else if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		err = fmt.Errorf("media delete failed with status code %d", httpResp.StatusCode)
	}
	if httpResp != nil {
		_ = httpResp.Body.Close()
	}
	return err
}
