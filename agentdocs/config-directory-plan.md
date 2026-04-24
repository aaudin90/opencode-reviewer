# План: переход от множества ENV к конфигурационной директории

## Цель

Упростить настройку `opencode-reviewer`: вместо большого числа переменных окружения пользователь указывает **один путь к директории конфигурации**, либо инструмент автоматически ищет `.opencodereview/` в корне проекта.

## UX и поведение

### Что задаёт пользователь

- Новый флаг CLI: `--config-dir PATH`.
- Новый ENV (опционально): `OR_CONFIG_DIR`.

### Как определяется директория конфигурации

Предлагаемый приоритет:

1. `--config-dir`
2. `OR_CONFIG_DIR`
3. `<project_dir>/.opencodereview`

Если директория не найдена, поведение настраиваемое:

- режим `strict` (по умолчанию): ошибка с подсказкой;
- режим `lenient`: продолжить со старыми источниками (TOML/ENV/default).

## Предлагаемая структура `.opencodereview/`

```text
.opencodereview/
  provider.json

  reviewer/
    agent.md
    messages/
      01-architecture.md
      02-style.md
    tools/
      submit_review.ts
    sub-agents/
      verifier.md

  finalizer/
    agent.md
    message.md
    tools/
      submit_final_review.ts
    sub-agents/
      verifier.md
```

## Маппинг директории на текущий Config

- `provider.json` → `opencode.provider_config_path`.
- `reviewer/agent.md` → `pipeline.review_agent_prompt_path`.
- `reviewer/messages/*.md` (лексикографически) → `pipeline.review_message_paths`.
- `finalizer/agent.md` → `pipeline.finalizer_prompt_path`.
- `finalizer/message.md` → `pipeline.finalizer_message_path`.
- `reviewer/sub-agents/*.md` → `pipeline.review_sub_agent_prompt_paths`.
- `finalizer/sub-agents/*.md` → `pipeline.finalizer_sub_agent_prompt_paths`.
- Scalar-настройки (`project_dir`, `[git]`, `[opencode]`, `[gitlab]`) задаются только через явный `--config` TOML или ENV.

Для кастомных тулов:

- `reviewer/tools/*.ts` и `finalizer/tools/*.ts` копировать/монтировать в workspace поверх дефолтных.
- Правило конфликта имён: файл из `.opencodereview` имеет приоритет над встроенным.

## Изменения в коде (по шагам)

### 1) Новый resolver для конфигурационной директории

Добавить пакет `internal/configdir`:

- `Resolve(cliConfigDir, envConfigDir, projectDir string) (string, Source, error)`
- `Discover(projectDir string) (string, bool)` — поиск `<project_dir>/.opencodereview`.

### 2) Расширить CLI и описание

В `cmd/reviewer/main.go`:

- добавить флаг `--config-dir`;
- добавить в help новый ENV `OR_CONFIG_DIR`;
- обновить секцию приоритетов.

### 3) Расширить загрузку Config

В `internal/config`:

- при наличии configDir автоматически подставлять дефолтные пути на файлы внутри директории.

### 4) Автоподстановка путей pipeline

Новый helper, например `internal/config/pathdefaults.go`:

- если `pipeline.review_agent_prompt_path` не задан, брать `reviewer/agent.md`;
- если `pipeline.review_message_paths` пустой, сканировать `reviewer/messages/*.md`;
- аналогично для finalizer и sub-agents.

### 5) Кастомные инструменты

В `internal/workspace`:

- добавить передачу `ToolOverrides` для reviewer/finalizer;
- копировать `tools/*.ts` в соответствующую workspace-директорию;
- сохранить текущие встроенные тулы как fallback.

### 6) Наблюдаемость и ошибки

Логировать источник путей:

- откуда взят configDir (CLI/ENV/auto-discovery),
- какие файлы обнаружены автоматически,
- какие fallback-ветки использованы.

Сообщения об ошибках делать action-oriented:

- где ожидался файл,
- как исправить,
- как явно переопределить через CLI/ENV.

## Совместимость и миграция

### Обратная совместимость

- Текущий режим `--config` + множество ENV продолжает работать.
- Новый режим config directory добавляется без breaking change.

### План миграции для пользователей

1. Создать `<repo>/.opencodereview`.
2. Перенести prompt/message/provider в структуру каталогов.
3. Удалить большинство `OR_*` из CI, оставив только секреты (`OR_GITLAB_TOKEN` и т.п.).
4. Включить `--config-dir .opencodereview` (или положиться на авто-поиск).

## Тест-план

### Unit

- resolver приоритетов (`--config-dir` > `OR_CONFIG_DIR` > auto-discovery),
- discovery `.opencodereview` при разных `project_dir`,
- автоподстановка путей pipeline,
- сортировка `messages/*.md` и `sub-agents/*.md`,
- конфликты кастомных и встроенных тулов.

### Integration

- end-to-end запуск с одной лишь `.opencodereview/` (без `OR_AGENT_PROMPT_PATH`, `OR_MESSAGE_PATHS`, `OR_FINALIZER_*`).
- регрессия старого режима (только TOML/ENV).

## Риски

- Неочевидный root проекта при запуске из поддиректории.
- Конфликт относительных путей между `--config` и `--config-dir`.
- Непредсказуемость при смешении inline TOML и auto-discovery.

## Решения по спорным местам

- Если одновременно указаны `--config` и `--config-dir`: 
  - `--config` отвечает только за TOML,
  - `--config-dir` отвечает за автофайлы prompts/messages/tools.
- Если в TOML задано inline-значение prompt/message, оно выше файлов из configDir.

## Предлагаемый rollout

1. **v1 (MVP)**: `--config-dir`, `OR_CONFIG_DIR`, auto-discovery, автофайлы prompts/messages.
2. **v2**: кастомные tools для reviewer/finalizer.
3. **v3**: мягкая депрекация части `OR_*`, документация миграции.
