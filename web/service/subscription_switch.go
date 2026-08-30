package service

func subscriptionAutoIDsEnabled() bool {
	enabled, err := (&SettingService{}).GetSubEnable()
	if err != nil {
		return true
	}
	return enabled
}
