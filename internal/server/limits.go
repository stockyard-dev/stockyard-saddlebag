package server

import "github.com/stockyard-dev/stockyard-saddlebag/internal/license"

type Limits struct {
	MaxFiles     int
	MaxSizeMB    int
	PasswordLock bool
	ExpiryLinks  bool
}

var freeLimits = Limits{MaxFiles: 10, MaxSizeMB: 50, PasswordLock: false, ExpiryLinks: true}
var proLimits = Limits{MaxFiles: 0, MaxSizeMB: 0, PasswordLock: true, ExpiryLinks: true}

func LimitsFor(info *license.Info) Limits {
	if info != nil && info.IsPro() { return proLimits }
	return freeLimits
}

func LimitReached(limit, current int) bool {
	if limit == 0 { return false }
	return current >= limit
}
