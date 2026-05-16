package profile

import "strings"

type Profile string

const (
	ProfileBeginner   Profile = "beginner"
	ProfileStandard   Profile = "standard"
	ProfilePowerUser  Profile = "power_user"
	ProfileExpert     Profile = "expert"
	ProfileLab        Profile = "lab"
	ProfileEnterprise Profile = "enterprise"
	ProfileCustom     Profile = "custom"
	ProfileUnknown    Profile = "unknown"
)

func NormalizeProfile(raw string) Profile {
	p := Profile(strings.ToLower(strings.TrimSpace(raw)))
	switch p {
	case ProfileBeginner, ProfileStandard, ProfilePowerUser, ProfileExpert,
		ProfileLab, ProfileEnterprise, ProfileCustom:
		return p
	default:
		return ProfileUnknown
	}
}

func normalizeProfileValue(p Profile) Profile {
	return NormalizeProfile(string(p))
}
