# SOAP

SOAP (Simple Object Access Protocol) — XML-based протокол для веб-сервисов, стандарт 1999 года. Сейчас встречается в legacy системах: банки, SAP, государственные сервисы, телеком.

---

## WSDL, конверт, заголовки, fault

### WSDL — описание сервиса

WSDL (Web Services Description Language) — XML-файл, который описывает:
- какие операции доступны
- типы данных запросов и ответов
- endpoint URL

```xml
<!-- Фрагмент WSDL -->
<definitions name="UserService" targetNamespace="http://example.com/users">
  
  <types>
    <schema>
      <element name="GetUserRequest">
        <complexType>
          <sequence>
            <element name="userId" type="string"/>
          </sequence>
        </complexType>
      </element>
      <element name="GetUserResponse">
        <complexType>
          <sequence>
            <element name="name" type="string"/>
            <element name="email" type="string"/>
          </sequence>
        </complexType>
      </element>
    </schema>
  </types>

  <portType name="UserPortType">
    <operation name="GetUser">
      <input message="GetUserRequest"/>
      <output message="GetUserResponse"/>
    </operation>
  </portType>
  
  <binding name="UserSOAPBinding" type="UserPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="GetUser">
      <soap:operation soapAction="http://example.com/GetUser"/>
    </operation>
  </binding>

  <service name="UserService">
    <port name="UserPort" binding="UserSOAPBinding">
      <soap:address location="http://api.example.com/users"/>
    </port>
  </service>
</definitions>
```

### SOAP конверт (Envelope)

Каждое сообщение — XML конверт:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope
    xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
    xmlns:usr="http://example.com/users">
    
  <!-- Header: авторизация, трейсинг, транзакции -->
  <soap:Header>
    <usr:AuthToken>Bearer abc123</usr:AuthToken>
    <usr:RequestID>req-456</usr:RequestID>
  </soap:Header>
  
  <!-- Body: основная нагрузка -->
  <soap:Body>
    <usr:GetUserRequest>
      <usr:userId>123</usr:userId>
    </usr:GetUserRequest>
  </soap:Body>
  
</soap:Envelope>
```

### SOAP Fault — ошибки

```xml
<soap:Body>
  <soap:Fault>
    <faultcode>soap:Client</faultcode>   <!-- Client = ошибка клиента, Server = ошибка сервера -->
    <faultstring>User not found</faultstring>
    <detail>
      <errorCode>USER_NOT_FOUND</errorCode>
      <userId>123</userId>
    </detail>
  </soap:Fault>
</soap:Body>
```

---

## Когда ещё встречается в 2025

- **Банки и финансы**: SWIFT, устаревшие банковские системы, процессинг платежей
- **Государственные сервисы**: ФНС, СМЭВ в России, множество европейских e-gov систем
- **SAP**: интеграции с SAP ERP
- **Телеком**: биллинговые системы, OSS/BSS
- **Healthcare**: HL7 интеграции, страховые системы

Новые системы SOAP не используют — переходят на REST/gRPC. Но при интеграции с legacy — никуда не деться.

---

## Как вызывать SOAP из Go

### Ручной подход (без генерации)

```go
const soapRequest = `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
               xmlns:usr="http://example.com/users">
    <soap:Body>
        <usr:GetUserRequest>
            <usr:userId>%s</usr:userId>
        </usr:GetUserRequest>
    </soap:Body>
</soap:Envelope>`

func getUser(ctx context.Context, userID string) error {
    body := fmt.Sprintf(soapRequest, userID)
    
    req, err := http.NewRequestWithContext(ctx, "POST",
        "http://api.example.com/users",
        strings.NewReader(body),
    )
    req.Header.Set("Content-Type", "text/xml; charset=utf-8")
    req.Header.Set("SOAPAction", "http://example.com/GetUser")
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // Парсинг XML ответа
    var envelope struct {
        Body struct {
            GetUserResponse struct {
                Name  string `xml:"name"`
                Email string `xml:"email"`
            } `xml:"GetUserResponse"`
            Fault *struct {
                Code   string `xml:"faultcode"`
                String string `xml:"faultstring"`
            } `xml:"Fault"`
        }
    }
    
    if err := xml.NewDecoder(resp.Body).Decode(&envelope); err != nil {
        return fmt.Errorf("decode: %w", err)
    }
    
    if envelope.Body.Fault != nil {
        return fmt.Errorf("soap fault %s: %s",
            envelope.Body.Fault.Code,
            envelope.Body.Fault.String)
    }
    
    return nil
}
```

### Генерация из WSDL: gowsdl

```bash
go install github.com/hooklift/gowsdl/cmd/gowsdl@latest
gowsdl -o gen/users.go http://api.example.com/users?wsdl
```

Генерирует Go structs и методы из WSDL. Качество генерации переменное — для сложных WSDL может требовать ручной правки.

---

## Сравнение с REST/gRPC: почему SOAP проиграл

| | SOAP | REST | gRPC |
|---|---|---|---|
| Формат | XML (verbose) | JSON | Protobuf |
| Overhead | Высокий (XML parsing) | Средний | Низкий |
| Human-readable | ⚠️ XML читабелен, но громоздкий | ✅ | ❌ binary |
| Стандарты | WS-Security, WS-* (много!) | HTTP conventions | Protobuf + HTTP/2 |
| Tooling 2025 | Устаревший | Богатый | Богатый |
| Гибкость | Низкая (жёсткая схема) | Высокая | Средняя |
| Browser | Через XMLHttpRequest | ✅ | ❌ |

**SOAP проиграл потому что:**
- XML в 5–20× больше JSON/Protobuf
- WS-* стандарты (WS-Security, WS-ReliableMessaging) сложны без понятной ценности
- REST оказался достаточно хорошим для большинства задач
- Tooling deградировал — лучшие генераторы заброшены

---

## Interview-ready answer

**Q: Встречал ли ты SOAP и как с ним работать из Go?**

SOAP встречается в legacy enterprise интеграциях: банки, государственные системы, SAP. Из Go — два подхода: ручное формирование XML конверта (strings.NewReader + правильные заголовки Content-Type и SOAPAction) с парсингом ответа через encoding/xml; или генерация из WSDL через gowsdl. Для сложных сервисов ручной подход надёжнее — генераторы часто плохо справляются с exotic WSDL. Главное — правильно обрабатывать SOAP Fault в Body ответа.
