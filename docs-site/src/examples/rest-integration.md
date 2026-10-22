# REST Integration

Calling external APIs from microflows -- GET, POST, authentication, and error handling.

## Simple GET

```sql
CREATE MICROFLOW Integration.FetchWebpage ()
RETURNS String AS $Content
BEGIN
  $Content = CALL REST SERVICE GET 'https://example.com/api/status' (
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Content;
END;

```

## GET with URL Parameters

```sql
CREATE MICROFLOW Integration.SearchProducts (
  $Query: String,
  $Page: Integer
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE GET 'https://api.example.com/search?q={1}&page={2}' WITH (
    {1} = urlEncode($Query),
    {2} = toString($Page)
  ) (
    Headers: ('Accept': 'application/json'),
    Timeout: 60,
  ) RETURNS String;
  RETURN $Response;
END;

```

## POST with JSON Body

```sql
CREATE MICROFLOW Integration.CreateCustomer (
  $Name: String,
  $Email: String
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE POST 'https://api.example.com/customers' (
    Headers: ('Content-Type': 'application/json'),
    Body: TEMPLATE '{{"name": "{1}", "email": "{2}"}' WITH (
      {1} = $Name,
      {2} = $Email
    ),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Response;
END;

```

## POST a Binary Body (File Upload)

Mendix models a binary request body on the **REST CALL activity**, not on a
consumed REST client document. The expression is the file document's `Contents`
member, and the content type goes on a header.

```sql
CREATE MICROFLOW Integration.UploadDocument (
  $Doc: Integration.UploadedFile
)
RETURNS Boolean AS $Ok
BEGIN
  DECLARE $Ok Boolean = false;
  $Response = CALL REST SERVICE POST 'https://api.example.com/documents' (
    Headers: ('ContentType': 'application/pdf'),
    Body: BINARY $Doc/Contents,
    Timeout: 300,
  ) RETURNS response;
  SET $Ok = $Response/StatusCode = 200;
  RETURN $Ok;
END;

```

`RETURNS response` binds a `System.HttpResponse`, so the microflow's own return
type must match what you do with it — returning `$Response` from a
`RETURNS String` microflow is `CE0117` at the end event.

`$Doc` must be a specialization of `System.FileDocument`.

A consumed REST **client document** has no binary body — its three body types
(`Rest$JsonBody`, `Rest$StringBody`, `Rest$ImplicitMappingBody`) are all textual
— so `Body: file from $Doc` in a `CREATE CONSUMED REST SERVICE` operation is refused as
**MDL-REST02**. Downloading is unaffected: `Response: file as $Doc` works.

## Basic Authentication

```sql
CREATE MICROFLOW Integration.FetchSecureData (
  $Username: String,
  $Password: String
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE GET 'https://api.example.com/secure/data' (
    Headers: ('Accept': 'application/json'),
    Authentication: BASIC (Username: $Username, Password: $Password),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Response;
END;

```

## Error Handling

Use `ON ERROR WITHOUT ROLLBACK` to catch failures and return a fallback instead of rolling back the transaction:

```sql
CREATE MICROFLOW Integration.SafeAPICall (
  $Url: String
)
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Success Boolean = false;

  $Response = CALL REST SERVICE GET $Url (
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
  ) RETURNS String
    ON ERROR WITHOUT ROLLBACK BEGIN
      LOG ERROR NODE 'Integration' 'API call failed: ' + $Url;
      RETURN false;
    END ERROR;

  SET $Success = true;
  RETURN $Success;
END;

```

## JSON Structure + Import Mapping

Map a JSON response to Mendix entities using a JSON structure and import mapping.

```sql
mdl 1;
-- Step 1: Define the JSON structure
CREATE JSON STRUCTURE Integration.JSON_Pet
  SAMPLE '{"id": 1, "name": "Fido", "status": "available"}';

-- Step 2: Create a non-persistent entity to hold the data
CREATE NON-PERSISTENT ENTITY Integration.PetResponse (
  PetId: Integer,
  Name: String,
  Status: String
);

-- Step 3: Create the import mapping
CREATE IMPORT MAPPING Integration.IMM_Pet
  WITH JSON STRUCTURE Integration.JSON_Pet
{
  CREATE Integration.PetResponse {
    PetId = id,
    Name = name,
    Status = status
  }
};

-- Step 4: Use in a microflow with IMPORT FROM MAPPING
-- $PetResponse = IMPORT FROM MAPPING Integration.IMM_Pet($JsonContent);
```

## Export Mapping (Entity → JSON)

Serialize entities back to JSON using an export mapping.

```sql
CREATE EXPORT MAPPING Integration.EMM_Pet
  WITH JSON STRUCTURE Integration.JSON_Pet
{
  Integration.PetResponse {
    id = PetId,
    name = Name,
    status = Status
  }
};

-- Use in a microflow with EXPORT TO MAPPING
-- $JsonOutput = EXPORT TO MAPPING Integration.EMM_Pet($PetResponse);
```

## Nested Mappings with Associations

Map nested JSON objects to multiple entities linked by associations.

```sql
CREATE IMPORT MAPPING Integration.IMM_Order
  WITH JSON STRUCTURE Integration.JSON_Order
{
  CREATE Integration.OrderResponse {
    OrderId = orderId KEY,
    CREATE Integration.OrderResponse_CustomerInfo/Integration.CustomerInfo = customer {
      Email = email,
      Name = name
    },
    CREATE Integration.OrderResponse_OrderItem/Integration.OrderItem = items {
      Sku = sku,
      Quantity = quantity,
      Price = price
    }
  }
};
```

## Consuming a REST API

### From an OpenAPI Spec (Recommended)

If the API has an OpenAPI 3.x spec (JSON or YAML), generate the REST client in one command.
A Swagger 2.0 spec is refused; convert it to OpenAPI 3 first (for example
`npx swagger2openapi swagger.json -o openapi.json`) and import the converted file.

```sql
mdl 1;
-- From a local file (relative to the .mpr file)
CREATE OR MODIFY CONSUMED REST SERVICE CapitalModule.CapitalAPI (
  OpenAPI: 'specs/capital.json'
);

-- From a URL
CREATE OR MODIFY CONSUMED REST SERVICE PetStoreModule.PetStoreAPI (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json'
);
```

Operations, path/query parameters, request bodies, response types, resource groups (from `tags`), and Basic auth are all derived automatically from the spec.

JSON responses are imported as `Response: JSON` (a string). The import does not yet create
entities for them: to get objects, create non-persistent entities and give the operation a
`Response: MAPPING Module.Entity { … }`, as shown under *Manual Definition* below.

**Preview without writing:**
```sql
DESCRIBE CONTRACT OPERATION FROM OPENAPI 'specs/capital.json';
```

### Manual Definition

Define a reusable REST client with typed operations using `CREATE CONSUMED REST SERVICE`. Each operation declares its method, path, optional parameters, headers, body, and response mapping.

```sql
-- Define a client for the orders API
CREATE CONSUMED REST SERVICE Integration.OrdersApi (
  BaseUrl: 'https://api.example.com/v1',
  Authentication: NONE
)
{
  OPERATION GetOrder (
    Method: GET,
    Path: '/orders/{id}',
    Parameters: ($id: String),
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
    Response: JSON AS $Result
  )

  OPERATION CreateOrder (
    Method: POST,
    Path: '/orders',
    Headers: ('Content-Type': 'application/json'),
    Body: MAPPING Integration.OrderRequest {
      customerId = CustomerId,
      totalAmount = TotalAmount,
      notes = Notes,
    },
    Response: MAPPING Integration.OrderResponse {
      Id = id,
      Status = status,
      CreatedAt = createdAt,
    }
  )
};
```

A header value is a template, like the path: `{Token}` is the operation parameter `Token`, which the operation must declare (`Parameters: ($Token: String)`, else CE7056). An authorization header is `Headers: ('Authorization': 'Bearer {Token}')`. The expression spelling `'Bearer ' + $Token` is deprecated (MDL-DEPR711) and `mxcli fmt --upgrade` rewrites it; it used to store only `Bearer `.

Use `CREATE OR MODIFY CONSUMED REST SERVICE` to replace an existing client's definition without dropping it first. It declares the **whole** operation list: an operation it does not list is removed, and the output names it. The statement below keeps `GetOrder` and removes `CreateOrder`:

```sql
CREATE OR MODIFY CONSUMED REST SERVICE Integration.OrdersApi (
  BaseUrl: 'https://api.example.com/v2',
  Authentication: BASIC (Username: 'apiuser', Password: 'secret')
)
{
  OPERATION GetOrder (
    Method: GET,
    Path: '/orders/{id}',
    Parameters: ($id: String),
    Headers: ('Accept': 'application/json'),
    Timeout: 60,
    Response: JSON AS $Result
  )
};
```

To add, change or remove one operation and leave the rest as stored — including operations made in Studio Pro, whose settings MDL cannot always express — use `ALTER CONSUMED REST SERVICE`. `MODIFY OPERATION` writes only the keys it lists:

```sql
ALTER CONSUMED REST SERVICE Integration.OrdersApi
  ADD OPERATION CancelOrder (
    Method: POST,
    Path: '/orders/{id}/cancel',
    Parameters: ($id: String),
    Response: NONE
  ),
  MODIFY OPERATION GetOrder (Timeout: 60),
  DROP OPERATION LegacyOrder,
  SET (BaseUrl: 'https://api.example.com/v2');
```

**Body types:** `JSON FROM $var`, `TEMPLATE '...'`, `MAPPING Entity { jsonField = Attr, ... }`
**Response types:** `JSON`, `STRING`, `FILE`, `STATUS` (each optionally `AS $var` — the variable is not stored; the call site binds it), `NONE`, `MAPPING Entity { Attr = jsonField, ... }`
**Authentication:** `NONE`, `BASIC (Username: '...', Password: '...')`

## Publishing a REST API

Create a published REST service with CRUD operations backed by microflows.

```sql
/** Order management API for the shop front end. */
CREATE PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API'
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetAllOrders;
    GET '{id}' MICROFLOW Module.PRS_GetOrderById;
    POST '' MICROFLOW Module.PRS_CreateOrder;
    PUT '{id}' MICROFLOW Module.PRS_UpdateOrder;
    DELETE '{id}' MICROFLOW Module.PRS_DeleteOrder;
  }
};
```

**Documentation:** the `/** … */` doc comment above `CREATE` is the service's documentation — what `lint` QUAL002 asks for. On `CREATE OR MODIFY` no comment keeps the stored text and `/** */` clears it. The OpenAPI description consumers read (`PublicDocumentation`) has no MDL spelling and is kept as stored.

**Operation paths:** Use empty string `''` for the root, `'{paramName}'` for path parameters. Do NOT start or end with `/`. Path parameters must match a microflow parameter name exactly (case-sensitive) — `'{id}'` binds the microflow's `$id`, whatever its type.

**Operation parameters** come from the microflow, the way Studio Pro derives them. Create the microflow before the service:

| Microflow parameter | Operation parameter |
|---|---|
| named in the path (`'{id}'`) | a path parameter |
| an object or a list | the body |
| `System.HttpRequest`, `System.HttpResponse` | none — they are the request and the response |
| anything else | a query parameter |

Each gets the microflow parameter's type. A header parameter, a renamed parameter or a description set in Studio Pro has no MDL spelling: `describe` notes it in a comment, and `CREATE OR MODIFY` / `ALTER` on that project keep it.

**Mappings and commit:** a body that is not a file document needs an import mapping, and a microflow returning an object or a list needs an export mapping:

```sql
mdl 1;
CREATE OR MODIFY PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API'
)
{
  RESOURCE 'orders' {
    -- COMMIT: Yes (the default without the clause) | YesWithoutEvents | No
    POST '' MICROFLOW Module.PRS_CreateOrder
      IMPORT MAPPING Module.IMM_Order EXPORT MAPPING Module.EMM_Order
      COMMIT YesWithoutEvents;
  }
};
```

### Authentication

`Authentication` sets how a caller authenticates — Studio Pro's **Requires
authentication** and its methods:

```sql
mdl 1;
CREATE OR MODIFY PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API',
  Authentication: (basic, session, microflow Module.PRS_Authenticate)
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetAllOrders;
  }
};
GRANT ACCESS ON PUBLISHED REST SERVICE Module.OrderAPI TO Module.User;

ALTER PUBLISHED REST SERVICE Module.OrderAPI SET (Authentication: (session));
ALTER PUBLISHED REST SERVICE Module.OrderAPI SET (Authentication: none);
```

| Method | Studio Pro |
|---|---|
| `basic` | Username and password |
| `session` | Active session |
| `microflow Module.Name` | Custom, with that authentication microflow |
| `none` (instead of a list) | Requires authentication: No |

- The methods are stored **in the order written**, as Studio Pro stores them in
  the order they were ticked; `describe` prints the stored order.
- **Left out**, `CREATE OR MODIFY` and `ALTER` keep the service's stored
  setting. A new service without it requires no authentication. `describe`
  does not print `none`.
- The **authentication microflow** returns `System.User` — empty means "not
  authenticated". An object parameter can only be a `System.HttpRequest` or a
  `System.HttpResponse`, matched by type; any other parameter is an
  **authentication parameter** of the service (below). `exec` and
  `check --references` refuse anything else as **MDL-REST04** (Mendix: CE0334,
  CE0336).
- With authentication on, the service needs at least one allowed module role
  (`GRANT ACCESS …`), or `mx check` reports CE0338. Custom authentication also
  needs app security on (CE6600). Roles without authentication are accepted.

#### Authentication parameters

A custom-authentication microflow can read values of the request as its own
parameters — a token in a header, a tenant in the query string. Each one is
bound in an optional list after the microflow (Studio Pro: the service's
**Parameters**, Mendix 7.17+):

```sql
mdl 1;
CREATE OR MODIFY MICROFLOW Module.PRS_Authenticate (
  $HttpRequest: System.HttpRequest, $Token: String, $Tenant: Integer, $Lang: String
)
RETURNS System.User AS $User
BEGIN
  RETRIEVE $User FROM System.User WHERE [Name = $Token] FIRST;
  RETURN $User;
END;

CREATE OR MODIFY PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API',
  Authentication: (
    basic,
    microflow Module.PRS_Authenticate (
      $Token: header 'X-Api-Token',
      $Tenant: query,
      $Lang: query 'lang' description 'UI language, defaults to en'
    )
  )
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetAllOrders;
  }
};
```

| Entry part | Meaning |
|---|---|
| `$Token` | The authentication microflow's parameter the value is passed to |
| `query` \| `header` | Where the value comes from; nothing else is accepted |
| `'X-Api-Token'` | The HTTP name, when it is not the parameter's own name |
| `description '…'` | Optional |

- The **data type** is never written: it is the microflow parameter's.
- A parameter **not listed** is a `query` parameter under its own name, so a
  microflow with `$Token: String` works with `microflow Module.PRS_Authenticate`
  alone. `System.HttpRequest` / `System.HttpResponse` are never listed. Without
  the service parameter Mendix reports CE0336 ("The microflow parameter
  ('Token') is not present in the custom authentication parameter list").
- **MDL-REST04** refuses an entry naming a parameter the microflow does not
  have, a request or response parameter, a kind other than `query` / `header`,
  and a parameter listed twice.
- The list follows the microflow: listed entries in the order written, then the
  unlisted ones in the microflow's order. Methods without `microflow` clear it.
  Left out with the rest of `Authentication`, the stored parameters are kept.
- `describe` prints the list only when the stored parameters are not what
  `microflow M.F` alone would write — a header, a renamed or described
  parameter, or another order.

### Multiple Resources

```sql
CREATE PUBLISHED REST SERVICE Module.CrmAPI (
  Path: 'rest/crm/v1',
  Version: '1.0.0',
  ServiceName: 'CRM API'
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetOrders;
  }
  RESOURCE 'customers' {
    GET '' MICROFLOW Module.PRS_GetCustomers;
  }
  RESOURCE 'orders/{orderId}/items' {
    GET '' MICROFLOW Module.PRS_GetOrderItems;
  }
};
```

### Update or Remove

```sql
-- Replace with new version
CREATE OR REPLACE PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v2',
  Version: '2.0.0',
  ServiceName: 'Order API v2'
) { ... };

-- Remove entirely
DROP PUBLISHED REST SERVICE Module.OrderAPI;
```

## Data Transformers (JSLT)

Data Transformers apply transformation steps (JSLT or XSLT) to JSON or XML payloads. Useful for reshaping API responses before import mapping, or normalising data from third-party sources. Requires Mendix 11.9+.

```sql
mdl 1;
-- Create a transformer that extracts key fields from a weather API response.
-- The SOURCE JSON defines a sample payload used for schema inference and testing.
CREATE DATA TRANSFORMER Integration.WeatherTransform
SOURCE JSON '{
  "latitude": 51.9,
  "longitude": 4.5,
  "timezone": "Europe/Amsterdam",
  "current": {
    "time": "2024-01-15T14:00",
    "temperature_2m": 12.8,
    "wind_speed_10m": 18.3,
    "weather_code": 3
  }
}'
{
  JSLT $$
{
  "lat":        .latitude,
  "lon":        .longitude,
  "timezone":   .timezone,
  "temp":       .current.temperature_2m,
  "wind_speed": .current.wind_speed_10m,
  "code":       .current.weather_code
}
  $$;
};

-- List all data transformers in the Integration module
LIST DATA TRANSFORMERS IN Integration;

-- Inspect a transformer (outputs a re-executable CREATE statement)
DESCRIBE DATA TRANSFORMER Integration.WeatherTransform;

-- Remove a transformer
DROP DATA TRANSFORMER Integration.WeatherTransform;
```

**Notes:**
- Steps execute in order; the output of each step feeds the next.
- `JSLT '...'` for short single-line expressions; `JSLT $$ ... $$` for multi-line.
- `XSLT $$ ... $$` is also supported for XML-to-XML transformations.
- Requires Mendix 11.9+. Use `LIST FEATURES` to confirm support before using.
