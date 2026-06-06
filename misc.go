// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

type respReachoutTimeoutLock struct {
	ReachoutTimeoutLock types.ReachoutTimeoutLock `json:"xwa2_fetch_account_reachout_timelock"`
}
type respGetNewChatMessageCappingInfo struct {
	MessageCappingInfo *types.NewChatMessageCappingInfo `json:"xwa2_message_capping_info"`
}

func (cli *Client) ReachoutTimeoutLock(ctx context.Context) (types.ReachoutTimeoutLock, error) {
	data, err := cli.sendMexIQ(ctx, fetchReachoutTimelockQuery, map[string]any{
		"input": map[string]any{},
	})

	var respData respReachoutTimeoutLock
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData.ReachoutTimeoutLock, err
}

func (cli *Client) GetNewChatMessageCappingInfo(ctx context.Context) (*types.NewChatMessageCappingInfo, error) {
	data, err := cli.sendMexIQ(ctx, queryNewChatMessageCappingInfo, map[string]any{
		"input": map[string]any{
			"type": "INDIVIDUAL_NEW_CHAT_MSG",
		},
	})
	var respData respGetNewChatMessageCappingInfo
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		} else if err == nil && respData.MessageCappingInfo == nil {
			err = fmt.Errorf("mex unexpected null response for new chat message capping info")
		}
	}
	return respData.MessageCappingInfo, err
}

func (cli *Client) BizIntegrity(ctx context.Context, queryInput []map[string]string) (types.BizIntegrity, error) {
	data, err := cli.sendMexIQ(ctx, fetchBizIntegrityQuery, map[string]any{
		"input": map[string]any{
			"query_input": queryInput,
			"telemetry": map[string]any{
				"context": "INTERACTIVE",
			},
		},
	})

	var respData types.BizIntegrity
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData, err
}
