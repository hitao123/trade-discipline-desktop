package market

import "strings"

func visibleETF(name string) bool {
	name = strings.TrimSpace(name)
	for _, marker := range []string{"债", "短融", "同业存单", "货币", "现金", "保证金", "理财金"} {
		if strings.Contains(name, marker) {
			return false
		}
	}
	return true
}
