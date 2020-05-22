// +build acceptance

package managers

type SuiteManager struct {
	out          func(format string, args ...interface{})
	results      map[string]interface{}
	cleanUpTasks map[string]func()
}

func NewSuiteManager(writer func(format string, args ...interface{})) *SuiteManager {
	return &SuiteManager{out: writer}
}

func (s *SuiteManager) CleanUp() error {
	for key, cleanUp := range s.cleanUpTasks {
		s.out("Running cleanup task '%s'\n", key)
		cleanUp()
	}

	return nil
}

func (s *SuiteManager) RegisterCleanUp(key string, cleanUp func()) {
	if s.cleanUpTasks == nil {
		s.cleanUpTasks = map[string]func(){}
	}

	s.cleanUpTasks[key] = cleanUp
}

func (s *SuiteManager) RunTaskOnce(key string, run func()) {
	if s.results == nil {
		s.results = map[string]interface{}{}
	}

	_, found := s.results[key]
	if !found {
		s.out("Running task '%s'\n", key)
		run()

		s.results[key] = true
	}
}
