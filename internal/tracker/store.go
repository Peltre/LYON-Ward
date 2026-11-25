// Saves the last memory state

package tracker

import "sync"

// Store guarda el último Snapshot en memoria. El sincronizador escribe desde su
// goroutine y el servidor web lee desde las suyas, así que todo acceso pasa por
// el candado.
type Store struct {
	mu   sync.RWMutex
	snap Snapshot
}

// NewStore devuelve un Store vacío y listo para usar. El valor cero del mutex
// ya es un candado abierto, así que no hay nada que inicializar.
func NewStore() *Store {
	return &Store{}
}

// Get devuelve una copia del snapshot actual.
// RLock deja pasar a varios lectores al mismo tiempo: solo bloquea si alguien
// está escribiendo.
func (s *Store) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Set reemplaza el snapshot completo. Nunca modificamos el que ya está
// guardado: siempre entra uno nuevo, construido desde cero.
func (s *Store) Set(snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap = snap
}

// SetError anota que la última sincronización falló, pero conserva los datos
// buenos que ya teníamos: más vale mostrar partidos de hace una hora con un
// aviso que una página vacía.
func (s *Store) SetError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Error = msg
}
