package users

import "context"

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) FindAll(ctx context.Context) ([]User, error) {
	return s.repository.FindAll(ctx)
}
