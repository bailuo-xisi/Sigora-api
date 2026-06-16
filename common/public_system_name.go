package common

import "strings"

var PublicSystemName = ""
var PublicLogo = ""

func GetPublicSystemName() string {
	if publicName := strings.TrimSpace(PublicSystemName); publicName != "" {
		return publicName
	}
	return SystemName
}

func GetPublicLogo() string {
	if publicLogo := strings.TrimSpace(PublicLogo); publicLogo != "" {
		return publicLogo
	}
	return Logo
}
