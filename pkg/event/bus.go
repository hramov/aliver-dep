package event

type HandlerFunc func(eventName string, payload any)

type Bus interface {
	Emit(eventName string, payload any)
	Subscribe(eventName string, handlers ...HandlerFunc)
}

type eventBus struct {
	h map[string][]HandlerFunc
}

// NewBus creates a new Bus.
//
// It returns a Bus interface.
func NewBus() Bus {
	return &eventBus{
		h: make(map[string][]HandlerFunc),
	}
}

// Subscribe adds one or more event handlers to be executed when a specific event occurs.
//
// eventName: the name of the event to subscribe to.
// handlers: one or more handler functions to be executed when the event occurs.
func (b *eventBus) Subscribe(eventName string, handlers ...HandlerFunc) {
	b.h[eventName] = append(b.h[eventName], handlers...)
}

// Emit sends the specified event with the given payload.
//
// eventName: the name of the event to emit.
// payload: the data to include with the event.
func (b *eventBus) Emit(eventName string, payload any) {
	for _, handler := range b.h[eventName] {
		go handler(eventName, payload)
	}
}
