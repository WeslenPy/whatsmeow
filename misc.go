// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"

	"go.mau.fi/whatsmeow/types"
)

type respReachoutTimeoutLock struct {
	ReachoutTimeoutLock types.ReachoutTimeoutLock `json:"a2_fetch_account_reachout_timelock"`
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

func (cli *Client) BizIntegrity(ctx context.Context, jids []string) (types.BizIntegrity, error) {
	data, err := cli.sendMexIQ(ctx, fetchBizIntegrityQuery, map[string]any{
		"input": map[string]any{
			"query_input": jids,
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
