package server

import "context"

func (s *stateStore) setProjectIcon(projectID int64, contentType string, data []byte) {
	if projectID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) == 0 || contentType == "" {
		delete(s.projectIcons, projectID)
		return
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	s.projectIcons[projectID] = projectIconState{
		ContentType: contentType,
		Data:        copied,
	}
}

func (s *stateStore) getProjectIcon(projectID int64) (projectIconState, bool) {
	if projectID <= 0 {
		return projectIconState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	icon, ok := s.projectIcons[projectID]
	if !ok || len(icon.Data) == 0 || icon.ContentType == "" {
		return projectIconState{}, false
	}
	copyData := make([]byte, len(icon.Data))
	copy(copyData, icon.Data)
	return projectIconState{
		ContentType: icon.ContentType,
		Data:        copyData,
	}, true
}

func (s *stateStore) GetProjectIcon(_ context.Context, projectID int64) (string, []byte, bool, error) {
	icon, ok := s.getProjectIcon(projectID)
	if !ok {
		return "", nil, false, nil
	}
	return icon.ContentType, icon.Data, true, nil
}
