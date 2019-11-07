package stringset

// TODO: test this file

func FromSlice(strings []string) map[string]interface{} {
	set := map[string]interface{}{}
	for _, s := range strings {
		set[s] = nil
	}
	return set
}

//func IsSuperset(super, sub []string) bool {
//	subset := toSet(sub)
//	for _, s := range super {
//		if _, ok := subset[s]; !ok {
//			return false
//		}
//	}
//	return true
//}
