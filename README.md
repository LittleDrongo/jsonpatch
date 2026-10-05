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
import "../jsonpatch"
```

### `Merge`

Сливает JSON patch с текущим значением. Правила берутся из тегов целевой модели.

```go
err := jsonpatch.Merge(&user, body)
```

### `MergeFrom`

Принимает структуру patch или `[]byte`. Структура одновременно задаёт список разрешённых полей; байты используют правила полной модели.

```go
var dto UpdateUserDTO
if err := json.Unmarshal(body, &dto); err != nil {
    return err
}
err := jsonpatch.MergeFrom(&user, dto)

// Или напрямую байты:
err = jsonpatch.MergeFrom(&user, body)
```

### `MergeRaw`

Сливает patch с исходным JSON и записывает результат в целевую структуру.

```go
err := jsonpatch.MergeRaw(&user, originalJSON, body)
```

### `IsNull`

Проверяет, содержит ли JSON-байтовый срез ровно `null`.

```go
if jsonpatch.IsNull(body) {
    // обработать null
}
```
