# Локальные цели разработки go-uptime.
# Типичный цикл: make fmt vet lint test / make migrate && make server
.PHONY: fmt vet lint lint-build test build migrate server

# Путь к кастомному бинарнику golangci-lint (собирается из .custom-gcl.yml).
CUSTOM_GCL := ./custom-gcl

# Привести весь дерево к стилю gofmt.
fmt:
	gofmt -w .

# Статический анализ компилятора (подозрительные конструкции).
vet:
	go vet ./...

# Собрать локальный бинарник golangci-lint с плагином NilAway.
lint-build:
	golangci-lint custom

# Запустить lint; при отсутствии ./custom-gcl сначала соберёт его (см. правило ниже).
lint: $(CUSTOM_GCL)
	$(CUSTOM_GCL) run ./...

# Пересобрать custom-gcl, если изменился .custom-gcl.yml или бинарника ещё нет.
$(CUSTOM_GCL): .custom-gcl.yml
	golangci-lint custom

# Все Go-тесты (нужна PostgreSQL и GO_UPTIME_* из .env / окружения).
test:
	go test ./...

# Собрать бинарник ./go-uptime в корне репозитория.
build:
	go build -o go-uptime .

# Применить встроенные Goose-миграции к БД из конфига.
migrate:
	go run . db-goose-migrate

# Запустить HTTP-сервер + фоновый worker.
server:
	go run . server
