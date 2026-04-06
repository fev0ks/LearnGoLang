package everything

import (
	"fmt"
	"regexp"
)

type Sample struct {
	Sample   string
	Expected bool
}

const (
	etisalat  = `^(etisalat(\d?|\d+).ae(\d?|\d+))$`
	etisalatc = "etisalat.ae"
	ims       = `^(ims(\d?|\d+))$`
	fixedlte  = `^((.+|.?)fixedlte)$`
	fims      = `^((.+|.?)fims)$`
	statSlots = "^[0-1][0-9]$|^[0-9]$"
)

func Try() {
	samples := []Sample{
		{"1", true},
		{"10", true},
		{"19", true},
		{"20", false},
		{"29", false},
		{"30", false},
	}
	checkSamples(samples, statSlots)
	//samples := []Sample{
	//	{"etisalat123.ae123", true},
	//	{"etisalat.ae123", true},
	//	{"etisalat.ae", true},
	//	{"etisalat.a", false},
	//	{"etisalat", false},
	//	{"etisalat.aeq", false},
	//	{"adm.etisalat.ae", false},
	//	{"qweetisalat.ae123", false},
	//}
	//checkSamples(samples, etisalat)
	////checkSamples(samples, etisalatc)
	//
	//samples = []Sample{
	//	{"ims123", true},
	//	{"ims1", true},
	//	{"ims", true},
	//	{"imsa", false},
	//	{"mims", false},
	//	{"1ims", false},
	//	{"qweims", false},
	//	{"im", false},
	//}
	//checkSamples(samples, ims)
	//
	//samples = []Sample{
	//	{"fixedlte", true},
	//	{"qfixedlte", true},
	//	{"1ewqfixedlte", true},
	//	{"fixedlteqwe", false},
	//	{"fixedlte23w", false},
	//	{"fixedlt", false},
	//	{"ixedlte", false},
	//}
	//checkSamples(samples, fixedlte)
	//
	//samples = []Sample{
	//	{"fims", true},
	//	{"qfims", true},
	//	{"1ewqfims", true},
	//	{"fimsqwe", false},
	//	{"fims23w", false},
	//	{"fim", false},
	//	{"ims", false},
	//}
	//checkSamples(samples, fims)
}

func checkSamples(samples []Sample, regexpTemplate string) {
	fmt.Println(regexpTemplate)
	for _, sample := range samples {
		match, err := regexp.MatchString(regexpTemplate, sample.Sample)
		if err != nil {
			fmt.Println(err)
		}
		if match != sample.Expected {
			fmt.Printf("Failed to check %s result: er %t - ar %t\n", sample.Sample, sample.Expected, match)
		} else {
			fmt.Printf("check '%s' result: %t\n", sample.Sample, match)
		}
	}
	fmt.Println()
}
