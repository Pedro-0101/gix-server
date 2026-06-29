package core

import "errors"

// Erros sentinela compartilhados entre store, service e a camada HTTP.
var (
	// ErrNotImplemented marca intents ainda não portadas (IA/embeddings vêm nas
	// próximas fases). Os handlers traduzem para 501.
	ErrNotImplemented = errors.New("core: não implementado")
	// ErrNotFound: recurso inexistente ou de outro usuário. Vira 404.
	ErrNotFound = errors.New("core: não encontrado")
)
