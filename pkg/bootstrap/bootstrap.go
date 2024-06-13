package bootstrap

type stateSyncer struct{}

func NewStateSyncer() (*stateSyncer, error) {
	ss := &stateSyncer{}
	err := ss.initControllers()
	if err != nil {
		return nil, err
	}
	return ss, nil
}

func (s *stateSyncer) Start() error {
	return nil
}

func (s *stateSyncer) initControllers() error {
	return nil
}
