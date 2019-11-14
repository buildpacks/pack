package stringset

// TODO: test this file

func FromSlice(strings []string) map[string]interface{} {
	set := map[string]interface{}{}
	for _, s := range strings {
		set[s] = nil
	}
	return set
}

// Join joins any number of slices removing duplicates
//
// NOTE: order is NOT preserved
func Join(strings ...[]string) []string {
	set := map[string]interface{}{}
	for _, slice := range strings {
		for _, s := range slice {
			set[s] = nil
		}
	}

	var values []string
	for k := range set {
		values = append(values, k)
	}
	return values
}

// TODO: document this
func Compare(strings1, strings2 []string) (extra []string, missing []string, common []string) {
	set1 := FromSlice(strings1)
	set2 := FromSlice(strings2)

	for s := range set1 {
		if _, ok := set2[s]; !ok {
			extra = append(extra, s)
			continue
		}
		common = append(common, s)
	}

	for s := range set2 {
		if _, ok := set1[s]; !ok {
			missing = append(missing, s)
		}
	}

	return extra, missing, common
}
