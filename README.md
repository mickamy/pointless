# pointless

A Go linter that suggests using value types instead of pointers for small structs.

The name is a pun: "pointer" + "less" = "pointless" (also meaning "unnecessary").

## Installation

```bash
go install github.com/mickamy/pointless@latest
```

## Usage

```bash
# Basic usage
pointless ./...

# Change threshold (default: 1024 bytes)
pointless -threshold 512 ./...
```

## What It Detects

### 1. Function Return Types

```go
// Warning: consider returning value instead of pointer
func GetUser() *User { ... }

// OK: may return nil
func FindUser(id int) *User {
    if notFound {
        return nil
    }
    return &user
}

// OK: struct is large (> 1024 bytes)
func GetData() *LargeData { ... }
```

### 2. Method Receivers

```go
// Warning: consider using value receiver
func (u *User) FullName() string {
    return u.FirstName + " " + u.LastName  // read-only
}

// OK: mutates receiver
func (u *User) SetName(name string) {
    u.Name = name
}
```

### 3. Pointer Slices

```go
// Warning: consider using []User instead of []*User
func GetUsers() []*User { ... }
users := make([]*User, 100)

// OK: uses nil as element
if users[i] == nil { ... }
```

### Not Checked: Function Arguments

```go
// Too difficult to determine intent
func (r *UserRepo) Update(u *User) error
func Process(u *User) error
```

## Suppressing Warnings

```go
// nolint:pointless
func GetUser() *User { ... }

// pointless:ignore
func GetUser() *User { ... }

// nolint
func GetUser() *User { ... }
```

## Configuration

Create `.pointless.yaml` or `.pointless.yml` in your project root:

```yaml
threshold: 1024  # bytes

exclude:
  - "*_test.go"
  - "vendor/**"
```

## CI Integration

```yaml
# .github/workflows/lint.yaml
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: pointless
        run: |
          go install github.com/mickamy/pointless@latest
          pointless ./...
```

## Why Prefer Value Types?

### Memory Layout

```
[]User (value slice):
┌────────┬────────┬────────┬────────┐
│ User 0 │ User 1 │ User 2 │ User 3 │  ← contiguous memory
└────────┴────────┴────────┴────────┘

[]*User (pointer slice):
┌────┬────┬────┬────┐
│ *  │ *  │ *  │ *  │  ← pointer array
└─┬──┴─┬──┴─┬──┴─┬──┘
  │    │    │    │
  ▼    ▼    ▼    ▼
┌────┐┌────┐┌────┐┌────┐  ← scattered allocations
│User││User││User││User│
└────┘└────┘└────┘└────┘
```

### Benefits of Value Types

1. **CPU cache efficiency** - Contiguous memory improves spatial locality
2. **Fewer allocations** - 1 allocation vs N+1 allocations
3. **Lower GC pressure** - Fewer pointers to track
4. **Immutability** - Prevents unintended mutations

### Why 1024 Bytes as Default Threshold?

- Go goroutine stacks start at 2KB and grow automatically
- A struct with ~50 fields typically uses 400-800 bytes
- Contiguous memory copies are efficiently handled by CPU (SIMD-optimized memcpy)
- Pointer indirection can cause cache misses, which may be more expensive than copying
- Slice/map fields only copy the header, not the underlying data

## License

[MIT](./LICENSE)
