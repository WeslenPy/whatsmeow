package types

type ReachoutTimeoutLock struct {
	IsActive            bool `json:"is_active"`
	TimeEnforcementEnds any  `json:"time_enforcement_ends"`
	EnforcementType     any  `json:"enforcement_type"`
}

type BizIntegrity struct {
	BizIntegrity any `json:"xwa2_fetch_wa_users"`
}

type NewChatMessageCappingInfo struct {
	TotalQuota          int    `json:"total_quota"`
	UsedQuota           int    `json:"used_quota"`
	CycleStartTimestamp string `json:"cycle_start_timestamp"`
	CycleEndTimestamp   string `json:"cycle_end_timestamp"`
	ServerSentTimestamp string `json:"server_sent_timestamp"`
	OTEStatus           string `json:"ote_status"`
	MVStatus            string `json:"mv_status"`
	CappingStatus       string `json:"capping_status"`
}
