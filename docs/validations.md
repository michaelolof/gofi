# Schema Validations

Gofi provides powerful, struct-tag based validation for your request schemas. It uses an internal validation engine largely compatible with standard ecosystem validators.

## How it Works

Validation rules are defined using the `validate` struct tag. Multiple rules can be separated by commas.

```go
type User struct {
    Name  string `validate:"required,min=2,max=50"`
    Email string `validate:"required,email"`
    Age   int    `validate:"gte=18"`
}
```

When you call `gofi.ValidateAndBind[T](c)`, Gofi validates the input data against these rules. If validation fails, it returns a structured error detailing which fields failed and why.

## Validation Only (No Binding)

If you only want to validate the request without binding the data to a struct (for example, if you want to inspect `c.Request()` manually afterwards), you can use `gofi.Validate(c)`.

```go
func(c gofi.Context) error {
    // Validates request against the route schema but does not return the binded struct.
    if err := gofi.Validate(c); err != nil {
        return err
    }

    // Access raw request data manually
    name := c.Request().URL.Query().Get("name")
    // ...
    return nil
}
```


## Supported Validators

Here is the complete list of supported validation tags in Gofi.

### Comparison & Length
| Tag | Description | Example |
| :--- | :--- | :--- |
| **`required`** | Field must be non-zero value. | `validate:"required"` |
| **`not_empty`** | Field must not be empty (strings, slices, maps). | `validate:"not_empty"` |
| **`len`** | Exact length (string/array/slice) or value (number). | `validate:"len=5"` |
| **`min`** | Minimum length (string/array/slice) or value (number). | `validate:"min=5"` |
| **`max`** | Maximum length (string/array/slice) or value (number). | `validate:"max=10"` |
| **`eq`** | Equals (==). | `validate:"eq=10"` |
| **`ne`** | Not Equals (!=). | `validate:"ne=0"` |
| **`gt`** | Greater Than (>). | `validate:"gt=10"` |
| **`lt`** | Less Than (<). | `validate:"lt=10"` |
| **`gte`** | Greater Than or Equal (>=). | `validate:"gte=10"` |
| **`lte`** | Less Than or Equal (<=). | `validate:"lte=10"` |
| **`oneof`** | Value must be one of the specified options (space separated). | `validate:"oneof=red green blue"` |

### String & Text Content
| Tag | Description |
| :--- | :--- |
| **`alpha`** | Alpha characters only. |
| **`alphanum`** | Alpha-numeric characters. |
| **`alphaunicode`** | Alpha unicode characters. |
| **`alphaunicodenum`** | Alpha-numeric unicode characters. |
| **`numeric`** | Numeric characters only. |
| **`number`** | Must be a valid number. |
| **`hexadecimal`** | Must be a valid hexadecimal string. |
| **`ascii`** | ASCII characters only. |
| **`printascii`** | Printable ASCII characters. |
| **`multibyte`** | Contains multi-byte characters. |
| **`lowercase`** | Must be lowercase. |
| **`uppercase`** | Must be uppercase. |
| **`contains`** | Must contain substring. |
| **`containsany`** | Must contain at least one of the characters in the param. |
| **`containsrune`** | Must contain the rune. |
| **`excludes`** | Must not contain substring. |
| **`excludesall`** | Must not contain any of the characters. |
| **`excludesrune`** | Must not contain the rune. |
| **`startswith`** | Must start with substring. |
| **`endswith`** | Must end with substring. |

### Format & Encoding
| Tag | Description |
| :--- | :--- |
| **`email`** | Valid email address. |
| **`json`** | Valid JSON string. |
| **`jwt`** | Valid JWT string. |
| **`base32`** | Valid Base32 string. |
| **`base64`** | Valid Base64 string. |
| **`base64url`** | Valid Base64 URL safe string. |
| **`base64rawurl`** | Valid Base64 Raw URL string. |
| **`html`** | Valid HTML tags. |
| **`html_encoded`** | Valid HTML encoded string. |
| **`url_encoded`** | Valid URL encoded string. |
| **`datauri`** | Valid Data URI Scheme. |
| **`semver`** | Valid Semantic Versioning 2.0.0. |

### Network & IP
| Tag | Description |
| :--- | :--- |
| **`ip`** | Valid IP address (v4 or v6). |
| **`ipv4`** | Valid IPv4 address. |
| **`ipv6`** | Valid IPv6 address. |
| **`ip_addr`** | Resolvable IP address. |
| **`ip4_addr`** | Resolvable IPv4 address. |
| **`ip6_addr`** | Resolvable IPv6 address. |
| **`cidr`** | Valid CIDR notation. |
| **`cidrv4`** | Valid IPv4 CIDR notation. |
| **`cidrv6`** | Valid IPv6 CIDR notation. |
| **`mac`** | Valid MAC address. |
| **`hostname`** | Valid Hostname (RFC 952). |
| **`hostname_rfc1123`**| Valid Hostname (RFC 1123). |
| **`hostname_port`** | Valid Hostname and Port. |
| **`fqdn`** | Full Qualified Domain Name. |
| **`tcp_addr`** | Resolvable TCP address. |
| **`tcp4_addr`** | Resolvable TCP4 address. |
| **`tcp6_addr`** | Resolvable TCP6 address. |
| **`udp_addr`** | Resolvable UDP address. |
| **`udp4_addr`** | Resolvable UDP4 address. |
| **`udp6_addr`** | Resolvable UDP6 address. |
| **`unix_addr`** | Resolvable Unix address. |
| **`uri`** | Valid URI. |
| **`url`** | Valid URL. |
| **`http_url`** | Valid HTTP/HTTPS URL. |
| **`fileUrl`** | Valid File URL. |
| **`urn_rfc2141`** | Valid URN (RFC 2141). |

### Identifiers
| Tag | Description |
| :--- | :--- |
| **`uuid`** / **`uuid4`** | Valid UUID (v4). |
| **`uuid3`** | Valid UUID (v3). |
| **`uuid5`** | Valid UUID (v5). |
| **`uuid_rfc4122`** | Valid UUID (RFC 4122). |
| **`ulid`** | Valid ULID. |
| **`ssn`** | Valid Social Security Number (US). |
| **`bic`** | Valid Business Identifier Code (SWIFT). |
| **`ein`** | Valid Employer Identification Number. |
| **`cve`** | Valid Common Vulnerabilities and Exposures ID. |
| **`isbn10`** | Valid ISBN 10. |
| **`isbn13`** | Valid ISBN 13. |
| **`issn`** | Valid ISSN. |

### Cryptography & Hashing
| Tag | Description |
| :--- | :--- |
| **`md4`** | Valid MD4 hash. |
| **`md5`** | Valid MD5 hash. |
| **`sha256`** | Valid SHA256 hash. |
| **`sha384`** | Valid SHA384 hash. |
| **`sha512`** | Valid SHA512 hash. |
| **`ripemd128`** | Valid RIPE MD128 hash. |
| **`ripemd160`** | Valid RIPE MD160 hash. |
| **`tiger128`** | Valid Tiger128 hash. |
| **`tiger160`** | Valid Tiger160 hash. |
| **`tiger192`** | Valid Tiger192 hash. |

### Time & Date
| Tag | Description |
| :--- | :--- |
| **`datetime`** | Parses as valid datatime. |
| **`timezone`** | Valid timezone. |
| **`cron`** | Valid cron expression. |

### File System
| Tag | Description |
| :--- | :--- |
| **`file`** | Valid file path that exists. |
| **`dir`** | Valid directory path that exists. |
| **`filepath`** | Valid file path characters. |

### Color
- `hexcolor` - Valid Hex color
- `rgb` - Valid RGB color
- `rgba` - Valid RGBA color
- `hsl` - Valid HSL color
- `hsla` - Valid HSLA color

### Finance
- `credit_card` - Valid credit card number
- `btc_addr` - Valid Bitcoin address
- `btc_addr_bech32` - Valid Bitcoin Bech32 address
- `eth_addr` - Valid Ethereum address
- `luhn_checksum` - Valid Luhn Checksum

### Geo & Location
- `latitude` - Valid Latitude
- `longitude` - Valid Longitude

### Database & Specialized
- `mongodb` - Valid MongoDB ObjectID
- `spicedb_id` - Valid SpiceDB ID
- `spicedb_permission` - Valid SpiceDB Permission
- `spicedb_type` - Valid SpiceDB Type

### Miscellaneous
- `boolean` - String "true"/"false" or boolean
- `default` - Sets a default value if empty
- `e164` - Valid E.164 phone number format

## Validation Utilities
Gofi also exposes two (2) lower-level validation functions that you can use anywhere in your application, not just within route handlers.

### `Validate(val any, rules string) error`
Validates a single value against a rule string.
```go
import "github.com/michaelolof/gofi/validators"

func checkEmail(email string) error {
    // Returns error if validation fails
    return validators.Validate(email, "required,email")
}

func main() {
    err := validators.Validate("not-an-email", "email")
    if err != nil {
        fmt.Println(err) // "validation failed..."
    }
}
```

### `ValidateStruct(s any) error`
Validates a struct's fields based on `validate` tags.

```go
type Config struct {
    Port int    `validate:"min=1024,max=65535"`
    Host string `validate:"required,hostname"`
}

func loadConfig() error {
    cfg := Config{Port: 80, Host: "localhost"}
    
    // Validates the struct custom logic
    if err := validators.ValidateStruct(&cfg); err != nil {
        return err
    }
    return nil
}
```

## Custom Validators

You can register your own custom validation rules. A custom validator must implement the `Validator` interface.

```go
type Validator interface {
    // Name returns the tag name to be used in structs
    Name() string
    // Rule returns the validation function generator
    Rule(c ValidatorContext) func(arg any) error
}
```

### Implementing a Custom Validator

Here is an example of a validator that checks if a string is "cool".

```go
import (
    "errors"
    "github.com/michaelolof/gofi"
)

type CoolValidator struct{}

func (v *CoolValidator) Name() string {
    return "is-cool"
}

// Rule generates a validation function.
// c contains context about the field being validated (like parameters).
func (v *CoolValidator) Rule(c gofi.ValidatorContext) func(arg any) error {
    return func(arg any) error {
        // Assert the type you expect
        s, ok := arg.(string)
        if !ok {
            // Skip validation if type doesn't match, or error out depending on logic
            return nil 
        }
        
        if s != "cool" {
            return errors.New("value must be 'cool'")
        }
        return nil
    }
}
```

### Registering the Validator

Register your custom validator with the router instance.

```go
func main() {
    r := gofi.NewServeMux()
    
    // Register the custom validator
    r.RegisterValidator(&CoolValidator{})
    
    // Usage in schema
    type MySchema struct {
        Status string `validate:"required,is-cool"`
    }
    
    // ...
}
```
