package app

const (
	CodeBadRequest      = "bad_request"
	CodeNotFound        = "not_found"
	CodeItemUnavailable = "item_unavailable"
)

type UserError struct {
	Code    string
	Message string
	ItemID  *int64
}

func (e *UserError) Error() string {
	return e.Message
}

func badRequest(message string) *UserError {
	return &UserError{Code: CodeBadRequest, Message: message}
}

func notFound(message string) *UserError {
	return &UserError{Code: CodeNotFound, Message: message}
}

func itemUnavailable(itemID int64) *UserError {
	id := itemID
	return &UserError{Code: CodeItemUnavailable, Message: "Позиции нет в наличии", ItemID: &id}
}

var (
	msgInvalidQty       = "Количество может быть от 1 до 20"
	msgInvalidSource    = "Недопустимый источник добавления"
	msgItemNotFound     = "Позиция не найдена"
	msgCartLineNotFound = "Позиция не найдена в корзине"
	msgUnknownEvent     = "Неизвестное событие"
	msgInvalidPayload   = "Некорректный payload события"
)
