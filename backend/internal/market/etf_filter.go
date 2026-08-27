package market

import "strings"

func IsEligibleETFName(name string) bool {
	name = strings.TrimSpace(name)
	for _, marker := range []string{"债", "短融", "同业存单", "货币", "现金", "保证金", "理财金"} {
		if strings.Contains(name, marker) {
			return false
		}
	}
	return name != ""
}

func IsEligibleETF(code, name string) bool {
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "511") {
		return false
	}
	return IsEligibleETFName(name)
}

func visibleETF(code, name string) bool { return IsEligibleETF(code, name) }
