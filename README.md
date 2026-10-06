# jsonpatch

Небольшой пакет для слияния JSON patch с Go-структурами. Правила полей задаются тегом `jsonpatch`.

## Теги

```go
type User struct {
    ID    int               `json:"id" jsonpatch:"readonly"`
    Name  string            `json:"name"`
    Secret string           `json:"secret" jsonpatch:"-"`
    Note  *string           `json:"note" jsonpatch:"null"`
    Tags  []string          `json:"tags" jsonpatch:"slice=append"`
    Items []Item            `json:"items" jsonpatch:"slice=merge,key=id"`
    Meta  map[string]string `json:"meta" jsonpatch:"map=merge"`
}
```

- `-` — игнорировать поле из patch.
- `readonly` — запрещать изменение поля.
- `null` — разрешать применение `null` к полю. По умолчанию `null` пропускается.
- `slice=replace` — заменить срез (режим по умолчанию).
- `slice=append` — добавить элементы в конец среза.
- `slice=merge,key=id` — слить элементы среза по значению JSON-поля `id`.
- `map=replace` — заменить map (режим по умолчанию).
- `map=merge` — слить map по ключам.

## Использование

```go
import "github.com/LittleDrongo/jsonpatch"
```

### `Merge`

Принимает patch любого типа и возвращает слитую структуру. Если patch — `[]byte`, он трактуется как сырой JSON и используются правила полной модели. Любой другой тип маршалится в JSON и одновременно задаёт список разрешённых полей (используются правила этого типа).

```go
// Сырой JSON — правила полной модели.
user, err := jsonpatch.Merge(&user, body)

// Структура patch — список разрешённых полей.
var dto UpdateUserDTO
if err := json.Unmarshal(body, &dto); err != nil {
    return err
}
user, err = jsonpatch.Merge(&user, dto)
```

### `Options`

- `ErrorOnReadOnly` — если `true`, попытка изменить поле с тегом `readonly` возвращает ошибку вместо тихого игнорирования.
- `AllowNull` — разрешить применение `null` ко всем полям (аналог тега `null`).
- `SliceMode`, `SliceKey` — как сливать массивы (значения по умолчанию для полей без тега).
- `MapMode` — как сливать map (значение по умолчанию для полей без тега).

```go
user, err := jsonpatch.Merge(&user, body, jsonpatch.Options{ErrorOnReadOnly: true})
```

## Пример

В каталоге `examples/` лежит небольшой HTTP-сервер на стандартном `net/http`:

```bash
go run ./examples
```

Эндпоинты:

- `GET /users/{id}` — получить пользователя.
- `PATCH /users/{id}` — частичное обновление сырым JSON; попытка изменить `id`/`role` (`readonly`) возвращает `422`.
- `PUT /users/{id}` — полное обновление сырым JSON; `id` и `role` защищены тегом `readonly` и молча игнорируются.

```bash
curl localhost:8080/users/1

# Ок — меняется только name.
curl -X PATCH localhost:8080/users/1 \
  -H 'Content-Type: application/json' \
  -d '{"name":"Carol"}'

# Ошибка 422 — role только для чтения.
curl -X PATCH localhost:8080/users/1 \
  -H 'Content-Type: application/json' \
  -d '{"role":"owner"}'

# Ок — id и role молча игнорируются (в PUT опция выключена).
curl -X PUT localhost:8080/users/1 \
  -H 'Content-Type: application/json' \
  -d '{"id":99,"name":"Dave","role":"owner"}'
```


