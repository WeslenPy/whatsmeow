package types

type ReachoutTimeoutLock struct {
	IsActive            bool `json:"is_active"`
	TimeEnforcementEnds any  `json:"time_enforcement_ends"`
}

type BizIntegrity struct {
	BizIntegrity any `json:"wa2_fetch_wa_users"`
}
