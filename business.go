// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	mathRand "math/rand/v2"
	"strconv"
	"strings"

	"go.mau.fi/libsignal/ecc"
	"google.golang.org/protobuf/proto"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waVnameCert"
	"go.mau.fi/whatsmeow/types"
)

func (cli *Client) SetBusinessName(ctx context.Context, name string) (*waBinary.Node, error) {

	details := &waVnameCert.VerifiedNameCertificate_Details{
		Serial:       proto.Uint64(mathRand.Uint64N(1_000_000_000_000_000) + 1),
		Issuer:       proto.String("smb:wa"),
		VerifiedName: proto.String(name),
	}

	detailsBytes, err := proto.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal verified name details: %w", err)
	}

	signature := ecc.CalculateSignature(
		ecc.NewDjbECPrivateKey(*cli.Store.IdentityKey.Priv),
		detailsBytes,
	)

	cert := &waVnameCert.VerifiedNameCertificate{
		Details:   detailsBytes,
		Signature: signature[:],
	}
	certBytes, err := proto.Marshal(cert)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal verified name certificate: %w", err)
	}

	return cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag:     "verified_name",
			Attrs:   waBinary.Attrs{"v": "2"},
			Content: certBytes,
		}},
	})
}

func (cli *Client) updateBusinessProfile(ctx context.Context, typeUpdate string, dataUpdate any) (*waBinary.Node, error) {
	return cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag:   "business_profile",
			Attrs: waBinary.Attrs{"v": "3", "mutation_type": "delta"},
			Content: []waBinary.Node{{
				Tag:     typeUpdate,
				Content: dataUpdate,
			}},
		}},
	})
}

func (cli *Client) SetEmailBusiness(ctx context.Context, email string) (*waBinary.Node, error) {
	return cli.updateBusinessProfile(ctx, "email", email)
}
func (cli *Client) SetWebsiteBusiness(ctx context.Context, website string) (*waBinary.Node, error) {
	return cli.updateBusinessProfile(ctx, "website", website)
}

func (cli *Client) SetDescriptionBusiness(ctx context.Context, description string) (*waBinary.Node, error) {
	return cli.updateBusinessProfile(ctx, "description", description)
}
func (cli *Client) SetAddressBusiness(ctx context.Context, address string) (*waBinary.Node, error) {
	return cli.updateBusinessProfile(ctx, "address", address)
}

func (cli *Client) SetBusinessHoursBusiness(ctx context.Context, timezone string, businessHours []types.BusinessHoursConfig) (*waBinary.Node, error) {
	configNodes := make([]waBinary.Node, 0, len(businessHours))
	for _, cfg := range businessHours {
		attrs := waBinary.Attrs{
			"day_of_week": cfg.DayOfWeek,
			"mode":        cfg.Mode,
		}
		if cfg.Mode == "specific_hours" {
			attrs["open_time"] = cfg.OpenTime
			attrs["close_time"] = cfg.CloseTime
		}
		configNodes = append(configNodes, waBinary.Node{
			Tag:   "business_hours_config",
			Attrs: attrs,
		})
	}

	return cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag:   "business_profile",
			Attrs: waBinary.Attrs{"v": "3", "mutation_type": "delta"},
			Content: []waBinary.Node{{
				Tag:     "business_hours",
				Attrs:   waBinary.Attrs{"timezone": timezone},
				Content: configNodes,
			}},
		}},
	})
}

func (cli *Client) SetCategoriesBusiness(ctx context.Context, categories []types.Category) (*waBinary.Node, error) {
	return cli.updateBusinessProfile(ctx, "categories", categories)
}

func (cli *Client) SetCoverPhotoBusiness(ctx context.Context, coverPhotoMedia *UploadResponse) (*waBinary.Node, error) {
	return cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag:   "business_profile",
			Attrs: waBinary.Attrs{"v": "3", "mutation_type": "delta"},
			Content: []waBinary.Node{{
				Tag: "cover_photo",
				Attrs: waBinary.Attrs{
					"id":    coverPhotoMedia.FBID,
					"op":    "update",
					"token": coverPhotoMedia.MetaHMAC,
					"ts":    strconv.FormatInt(coverPhotoMedia.Timestamp, 10),
				},
			}},
		}},
	})
}

func (cli *Client) RemoveCoverPhotoBusiness(ctx context.Context, coverPhotoMedia *UploadResponse) (*waBinary.Node, error) {
	return cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag:   "business_profile",
			Attrs: waBinary.Attrs{"v": "3", "mutation_type": "delta"},
			Content: []waBinary.Node{{
				Tag: "cover_photo",
				Attrs: waBinary.Attrs{
					"op": "delete",
					"id": coverPhotoMedia.FBID,
				},
			}},
		}},
	})
}

func (cli *Client) GetCatalogProductsBusiness(ctx context.Context, jid types.JID, limit int, cursor string) (*waBinary.Node, error) {
	if jid.IsEmpty() {
		if cli.Store.ID == nil {
			return nil, ErrNotLoggedIn
		}
		jid = cli.Store.ID.ToNonAD()
	}

	if limit <= 0 {
		limit = 10
	}

	queryParamNodes := []waBinary.Node{
		{Tag: "limit", Content: []byte(strconv.Itoa(limit))},
		{Tag: "width", Content: []byte("100")},
		{Tag: "height", Content: []byte("100")},
	}
	if cursor != "" {
		queryParamNodes = append(queryParamNodes, waBinary.Node{
			Tag:     "after",
			Content: []byte(cursor),
		})
	}

	return cli.sendIQ(ctx, infoQuery{
		Type:      iqGet,
		To:        types.ServerJID,
		Namespace: "w:biz:catalog",
		Content: []waBinary.Node{{
			Tag: "product_catalog",
			Attrs: waBinary.Attrs{
				"jid":               jid,
				"allow_shop_source": "true",
			},
			Content: queryParamNodes,
		}},
	})
}

// CreateProductBusiness creates a new product in the business catalog.
func (cli *Client) CreateProductBusiness(ctx context.Context, create types.ProductCreate) (*types.Product, error) {
	create, err := cli.uploadingNecessaryImagesOfProduct(ctx, create)
	if err != nil {
		return nil, err
	}

	createNode := toProductNode("", create)

	result, err := cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz:catalog",
		Content: []waBinary.Node{{
			Tag:   "product_catalog_add",
			Attrs: waBinary.Attrs{"v": "1"},
			Content: []waBinary.Node{
				createNode,
				{Tag: "width", Content: []byte("100")},
				{Tag: "height", Content: []byte("100")},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	addNode := result.GetChildByTag("product_catalog_add")
	productNode := addNode.GetChildByTag("product")
	return parseProductNode(&productNode), nil
}

// DeleteProductBusiness deletes the given products from the business catalog and
// returns the number of products that were actually deleted.
func (cli *Client) DeleteProductBusiness(ctx context.Context, productIDs []string) (int, error) {
	productNodes := make([]waBinary.Node, 0, len(productIDs))
	for _, id := range productIDs {
		productNodes = append(productNodes, waBinary.Node{
			Tag: "product",
			Content: []waBinary.Node{{
				Tag:     "id",
				Content: []byte(id),
			}},
		})
	}

	result, err := cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz:catalog",
		Content: []waBinary.Node{{
			Tag:     "product_catalog_delete",
			Attrs:   waBinary.Attrs{"v": "1"},
			Content: productNodes,
		}},
	})
	if err != nil {
		return 0, err
	}

	deleteNode := result.GetChildByTag("product_catalog_delete")
	return deleteNode.AttrGetter().OptionalInt("deleted_count"), nil
}

// UpdateProductBusiness updates an existing product in the business catalog.
func (cli *Client) UpdateProductBusiness(ctx context.Context, productID string, update types.ProductCreate) (*types.Product, error) {
	update, err := cli.uploadingNecessaryImagesOfProduct(ctx, update)
	if err != nil {
		return nil, err
	}

	editNode := toProductNode(productID, update)

	result, err := cli.sendIQ(ctx, infoQuery{
		Type:      iqSet,
		To:        types.ServerJID,
		Namespace: "w:biz:catalog",
		Content: []waBinary.Node{{
			Tag:   "product_catalog_edit",
			Attrs: waBinary.Attrs{"v": "1"},
			Content: []waBinary.Node{
				editNode,
				{Tag: "width", Content: []byte("100")},
				{Tag: "height", Content: []byte("100")},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	editResultNode := result.GetChildByTag("product_catalog_edit")
	productNode := editResultNode.GetChildByTag("product")
	return parseProductNode(&productNode), nil
}

// GetCollectionsBusiness fetches the catalog collections of the given business.
//
// If jid is empty, the logged-in account's own JID is used. A limit of <= 0 defaults to 51.
func (cli *Client) GetCollectionsBusiness(ctx context.Context, jid types.JID, limit int) ([]types.Collection, error) {
	if jid.IsEmpty() {
		if cli.Store.ID == nil {
			return nil, ErrNotLoggedIn
		}
		jid = cli.Store.ID.ToNonAD()
	}

	if limit <= 0 {
		limit = 51
	}
	limitStr := strconv.Itoa(limit)

	result, err := cli.sendIQ(ctx, infoQuery{
		Type:      iqGet,
		To:        types.ServerJID,
		Namespace: "w:biz:catalog",
		Content: []waBinary.Node{{
			Tag:   "collections",
			Attrs: waBinary.Attrs{"biz_jid": jid, "smax_id": "35"},
			Content: []waBinary.Node{
				{Tag: "collection_limit", Content: []byte(limitStr)},
				{Tag: "item_limit", Content: []byte(limitStr)},
				{Tag: "width", Content: []byte("100")},
				{Tag: "height", Content: []byte("100")},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	return parseCollectionsNode(result), nil
}

// parseCollectionsNode parses a <collections> response into a list of collections.
func parseCollectionsNode(node *waBinary.Node) []types.Collection {
	if node == nil {
		return nil
	}
	collectionsNode := node.GetChildByTag("collections")
	collectionNodes := collectionsNode.GetChildrenByTag("collection")
	collections := make([]types.Collection, 0, len(collectionNodes))
	for _, collectionNode := range collectionNodes {
		productNodes := collectionNode.GetChildrenByTag("product")
		products := make([]types.Product, 0, len(productNodes))
		for _, productNode := range productNodes {
			if parsed := parseProductNode(&productNode); parsed != nil {
				products = append(products, *parsed)
			}
		}
		collections = append(collections, types.Collection{
			ID:       nodeStringContent(collectionNode.GetChildByTag("id")),
			Name:     nodeStringContent(collectionNode.GetChildByTag("name")),
			Products: products,
			Status:   parseStatusInfo(collectionNode),
		})
	}
	return collections
}

// parseStatusInfo parses the <status_info> child of the given node.
func parseStatusInfo(node waBinary.Node) types.CatalogStatus {
	statusInfoNode := node.GetChildByTag("status_info")
	return types.CatalogStatus{
		Status:    nodeStringContent(statusInfoNode.GetChildByTag("status")),
		CanAppeal: nodeStringContent(statusInfoNode.GetChildByTag("can_appeal")) == "true",
	}
}

// uploadingNecessaryImagesOfProduct uploads any product images that aren't already
// hosted on WhatsApp's servers, replacing them with the resulting WhatsApp URLs.
func (cli *Client) uploadingNecessaryImagesOfProduct(ctx context.Context, product types.ProductCreate) (types.ProductCreate, error) {
	if len(product.Images) == 0 {
		return product, nil
	}

	uploaded := make([]types.ProductImage, len(product.Images))
	for i, img := range product.Images {
		if img.URL != "" && strings.Contains(img.URL, ".whatsapp.net") {
			uploaded[i] = types.ProductImage{URL: img.URL}
			continue
		}
		if len(img.Data) == 0 {
			return product, fmt.Errorf("product image %d has no whatsapp.net URL and no data to upload", i)
		}
		resp, err := cli.UploadProductCatalogImage(ctx, img.Data)
		if err != nil {
			return product, fmt.Errorf("failed to upload product image %d: %w", i, err)
		}
		url := resp.URL
		if url == "" {
			url = "https://mmg.whatsapp.net" + resp.DirectPath
		}
		uploaded[i] = types.ProductImage{URL: url}
	}
	product.Images = uploaded
	return product, nil
}

// toProductNode builds the <product> binary node for a catalog create/update request.
// When productID is empty (creation), no <id> child is added.
func toProductNode(productID string, product types.ProductCreate) waBinary.Node {
	attrs := waBinary.Attrs{}
	var content []waBinary.Node

	if productID != "" {
		content = append(content, waBinary.Node{Tag: "id", Content: []byte(productID)})
	}
	if product.Name != "" {
		content = append(content, waBinary.Node{Tag: "name", Content: []byte(product.Name)})
	}
	if product.Description != "" {
		content = append(content, waBinary.Node{Tag: "description", Content: []byte(product.Description)})
	}
	if product.RetailerID != "" {
		content = append(content, waBinary.Node{Tag: "retailer_id", Content: []byte(product.RetailerID)})
	}
	if len(product.Images) > 0 {
		imageNodes := make([]waBinary.Node, 0, len(product.Images))
		for _, img := range product.Images {
			imageNodes = append(imageNodes, waBinary.Node{
				Tag: "image",
				Content: []waBinary.Node{{
					Tag:     "url",
					Content: []byte(img.URL),
				}},
			})
		}
		content = append(content, waBinary.Node{Tag: "media", Content: imageNodes})
	}
	if product.Price != 0 {
		content = append(content, waBinary.Node{Tag: "price", Content: []byte(strconv.FormatInt(product.Price, 10))})
	}
	if product.Currency != "" {
		content = append(content, waBinary.Node{Tag: "currency", Content: []byte(product.Currency)})
	}
	attrs["is_hidden"] = strconv.FormatBool(product.IsHidden)

	return waBinary.Node{Tag: "product", Attrs: attrs, Content: content}
}

// parseProductNode parses a <product> binary node into a Product.
func parseProductNode(productNode *waBinary.Node) *types.Product {
	if productNode == nil {
		return nil
	}

	mediaNode := productNode.GetChildByTag("media")
	imageNode := mediaNode.GetChildByTag("image")
	statusInfoNode := productNode.GetChildByTag("status_info")

	price, _ := strconv.ParseInt(nodeStringContent(productNode.GetChildByTag("price")), 10, 64)

	return &types.Product{
		ID:           nodeStringContent(productNode.GetChildByTag("id")),
		Name:         nodeStringContent(productNode.GetChildByTag("name")),
		Description:  nodeStringContent(productNode.GetChildByTag("description")),
		RetailerID:   nodeStringContent(productNode.GetChildByTag("retailer_id")),
		URL:          nodeStringContent(productNode.GetChildByTag("url")),
		Price:        price,
		Currency:     nodeStringContent(productNode.GetChildByTag("currency")),
		IsHidden:     productNode.AttrGetter().OptionalString("is_hidden") == "true",
		ReviewStatus: nodeStringContent(statusInfoNode.GetChildByTag("status")),
		ImageURLs: types.ProductImageURLs{
			Requested: nodeStringContent(imageNode.GetChildByTag("request_image_url")),
			Original:  nodeStringContent(imageNode.GetChildByTag("original_image_url")),
		},
	}
}

// nodeStringContent returns the textual content of a binary node, handling both
// []byte and string content representations.
func nodeStringContent(node waBinary.Node) string {
	switch content := node.Content.(type) {
	case []byte:
		return string(content)
	case string:
		return content
	default:
		return ""
	}
}
