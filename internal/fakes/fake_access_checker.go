package fakes

type FakeAccessChecker struct {
	RegistriesToFail []string
}

func NewFakeAccessChecker() *FakeAccessChecker {
	return &FakeAccessChecker{}
}

func (f *FakeAccessChecker) Check(repo string) bool {
	for _, toFail := range f.RegistriesToFail {
		if toFail == repo {
			return false
		}
	}

	return true
}
