package stringset

// TODO: test this file

func FromSlice(strings []string) map[string]interface{} {
	set := map[string]interface{}{}
	for _, s := range strings {
		set[s] = nil
	}
	return set
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
			extra = append(missing, s)
		}
	}
	
	return extra, missing, common
}
