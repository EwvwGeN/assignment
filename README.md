# Оглавление

- [Запуск](#запуск)
- [Объекты](#объекты)
- [Запросы](#запросы)
    - [Post](#post)
    - [Get](#get)
    - [Put](#put)
    - [Delete](#delete)
<br/><br/>

## Запуск
При старте api сервера задается режим работы: с файлом конфигурации и без. Для работы с файлом конфигурации при запуске указывается флаг “-c”.

В случае если флаг не был указана все необходимые настройки берутся из .env файла, а те настройки, которых нет в окружении выставляются в значение по умолчанию.

Файл конфигурации имеет следующие настройки
```
api_host: "0.0.0.0"
api_port: "8080"
db_host: "0.0.0.0"
db_port: "6534"
db_name: "testdb"
collection_name: "documents"
nesting_level: 2
cache_life_time_m: 15
cache_cleaning_interval_m: 10
```

Где
- api_host, api_port — адрес, по которому будет работать api
- db_host, db_port, db_name, collection_name — данные для подключения к reindexer (хост, порт, имя подключаемой базы данных и коллекция внутри бд соответственно)
- nesting_level — максимальный допустимый уровень вложенности документов
- cache_life_time_m, cache_cleaning_interval_m — время жизни кеша и интервал очистки.

Также в проекте лежат готовые решения для Docker. Как и запуск исключительно сервера в контейнере (Dockerfile), так и запуск одновременно двух контейнеров с сервером и базой данных (Docker-compose). Для этих решений так же предполагается возможность использования конфига (аргумент ISCNF). Однако указывать это нужно на этапе сборки.
```
docker-compose build --build-arg ISCNF=-c
```
Решение для докера потребует предварительной компиляции
```
CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server/
```
<br/><br/>

## Объекты
Объектом базы данных является документ, обладающий следующей структурой:
```go
type Document struct {
    Id        int64
    ParentId  int64
    Depth     int
    Sort      int
    Body      string
    ChildList []int64
}
```

Поля `Id`, `ParentId`, `Depth`, `ChildList`, `Sort` являются системными, поле Body содержит непосредственно данные документа. Документ может содержать бесконечное количество несистемных полей.

Поля, доступные для изменения, прописываются в отдельной структуре:
```go
type AllowedField struct {
    Sort      int
    Body      string
    ChildList []int64
}
```

Для отображения полного документа используется следующая структура:
```go
type BigDocument struct {
    Id        int64
    Body      string
    ChildList []BigDocument
}
```
<br/><br/>
## Запросы

API реализует только http запросы. Доступными являются следующие методы: POST, GET, PUT, DELETE. Системные поля в запросах не могут быть изменены (за исключением `ChildList`). Лишние поля json не обрабатываются и ошибок не вызывают.
<br/><br/>

#### POST
Запрос осуществляется по пути /docs. В теле запроса может не быть информации, тогда будет создан новый пустой документ.
Пример запроса:
```
POST /docs HTTP/1.1
Content-Type: application/json
 
{
    "Body": "Body of new-created document"
}
```
Ответ:
```
HTTP/1.1 201 Created
Content-Type: application/json; charset=utf-8
Date: Mon, 26 Jun 2023 15:17:39 GMT
Content-Length: 131
 
{
    "Id": 44,
    "ParentId": 0,
    "Depth": 0,
    "Sort": 0,
    "Body": "Body of new-created document",
    "ChildList": []
}
```
<br/><br/>

#### GET
Запросы осуществляются по путям:
- `/docs` — вывод всех документов;
- `/docs/:id` — вывод документа с определенным id;
- `/big-docs` — вывод всех полных документов;
- `/big-docs/:id` — вывод полного документа с определнным id. Если указанный id не является верхним выведется верхний документ родитель.

Для получения списка документов предусмотрена пагинация со следующими параметрами:
- `page` — номер страницы,
- `limit` — количество документов, выводимых на одной странице.

Также при получении полного документа, вложенные документы первого уровня сортируются в обратном порядке по полю `sort`.

Пример запроса:
```
GET /big-docs?page=2&limit=1 HTTP/1.1
```
Ответ:
```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Mon, 26 Jun 2023 15:24:10 GMT
Content-Length: 426

{
    "Id": 36,
    "Sort": 0,
    "Body": "parent num 2",
    "ChildList": [
        {
            "Id": 35,
            "Sort": 0,
            "Body": "updated document",
            "ChildList": [
                {
                    "Id": 43,
                    "Sort": 0,
                    "Body": "Body of new-created document",
                    "ChildList": null
                }
            ]
        }
    ]
}
```
<br/><br/>

#### PUT
Запрос осуществляется по пути `/docs`. В теле запроса нужно указать Id и поля, которые необходимо обновить. Для обновления доступны поля `ChildList`, `Sort`, а также все несистемные. Ответ при обновлении будет либо ошибка, либо сообщение об успешном обновлении.
Пример запроса:
```
PUT /docs HTTP/1.1
Content-Type: application/json

{
    "Id": 40,
    "Sort": 3
}
```
Ответ:
```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Mon, 26 Jun 2023 15:30:00 GMT
Content-Length: 23

{
    "message": "ok"
}
```
<br/><br/>

#### DELETE
Запрос осуществляется по пути `/docs/:id`. При удалении документа все дочерние документы также удаляются, при этом обновляется глубина у всех документов верхнего уровня.

Пример запроса:
```
DELETE /docs/36 HTTP/1.1
```
Ответ:
```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Mon, 26 Jun 2023 15:31:57 GMT
Content-Length: 23
 
{
    "message": "ok"
}
```
