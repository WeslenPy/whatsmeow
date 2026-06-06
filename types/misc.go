package types

type ReachoutTimeoutLock struct {
	IsActive            bool `json:"is_active"`
	TimeEnforcementEnds any  `json:"time_enforcement_ends"`
	EnforcementType     any  `json:"enforcement_type"`
}

type BizIntegrity struct {
	BizIntegrity any `json:"xwa2_fetch_wa_users"`
}
