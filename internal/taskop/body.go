package taskop

import "github.com/alkulagin-creator/tt/internal/vault"

// SetBody переписывает тело таски (весь markdown после фронтматтера) с той
// же защитой от конкурентной правки, что у SetIfVersion: baseVersion
// обязателен и сверяется с vault.Version перед записью.
//
// В отличие от Set, тело не входит в белый список полей фронтматтера — это
// не YAML-ключ, а произвольный markdown, поэтому проверка ровно одна: таска
// существует, разбирается и не была изменена с момента чтения.
func SetBody(vaultDir, id, body, baseVersion string) (Result, error) {
	if baseVersion == "" {
		return Result{}, failf(KindBadValue, "не указан baseVersion")
	}
	_, _, task, err := locate(vaultDir, id)
	if err != nil {
		return Result{}, err
	}
	if err := checkVersion(task.Path, baseVersion); err != nil {
		return Result{}, err
	}
	if err := vault.SetBody(task.Path, body); err != nil {
		return Result{}, failf(KindWrite, "%w", err)
	}
	return reread(task, task.Status, "")
}
