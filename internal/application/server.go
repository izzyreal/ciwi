package application

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ServerInfoSource interface {
	ServerInfo(context.Context) (domain.ServerInfo, error)
}

type ServerQueries struct {
	source ServerInfoSource
}

func NewServerQueries(source ServerInfoSource) *ServerQueries {
	return &ServerQueries{source: source}
}

func (q *ServerQueries) GetServerInfo(ctx context.Context) (domain.ServerInfo, error) {
	if q == nil || q.source == nil {
		return domain.ServerInfo{}, NewError(ErrorUnavailable, "server information unavailable", nil)
	}
	info, err := q.source.ServerInfo(ctx)
	if err != nil {
		return domain.ServerInfo{}, WrapInternal("get server information", err)
	}
	return info, nil
}
