package notify

import (
	"context"
	"log"
)

// Notifier — абстракция над отправкой уведомлений (ТЗ, раздел 4.5: о
// перебитой ставке, о результате аукциона). Реальный провайдер (SendGrid/
// Resend, см. ТЗ раздел 7) подключается отдельной реализацией этого
// интерфейса; на этапе MVP используется LogNotifier.
type Notifier interface {
	Send(ctx context.Context, to, subject, body string) error
}

// LogNotifier просто пишет уведомления в лог — заглушка для дев-окружения,
// где нет реального email-провайдера. Поведенчески ничего не роняет и не
// блокирует основной поток (вызывающий код всё равно не должен считать
// недоставленное уведомление фатальной ошибкой заявки/ставки).
type LogNotifier struct{}

func (LogNotifier) Send(_ context.Context, to, subject, body string) error {
	log.Printf("notify: to=%s subject=%q body=%q", to, subject, body)
	return nil
}
