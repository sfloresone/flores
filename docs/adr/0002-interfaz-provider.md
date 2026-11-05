# 0002. Interfaz del proveedor de modelos

- **Estado:** Aceptada. La pregunta abierta (SDK o `net/http`) se cerró a favor de `net/http` propio; ver abajo.
- **Fecha:** 2026-11-05

## Contexto

La abstracción del proveedor es lo que hace a flores agnóstico al modelo. Dos
referencias:

- **[byoharness, capítulo 3](https://www.byoharness.dev/es/chapters/03-the-provider-interface.html).**

  ```go
  type Provider interface {
      Send(ctx context.Context, messages []Message, tools []ToolDef) (Response, error)
      Model() string
      SetModel(name string)
  }
  ```

  - Acierta en lo esencial: tipos propios (`Message`, `Block`, `ToolDef`), ningún
    tipo del SDK sale del adaptador, y un fichero por proveedor. Además ordena las
    tools por nombre para que la caché de prompt reciba bytes estables.
  - Le faltan cosas: no tiene streaming, devuelve el error del SDK tal cual (sin
    clasificar), no informa del consumo de tokens, y el modelo es estado mutable
    (`SetModel`).

- **[`charm.land/fantasy`](https://github.com/charmbracelet/fantasy)**, que es lo
  que usa crush.
  - Interfaz `LanguageModel` con `Generate`, `Stream`, `GenerateObject` y
    `StreamObject`.
  - El streaming es un iterador `iter.Seq[StreamPart]` con unos 15 tipos de evento.
  - Tiene opciones por proveedor y un agente con más de 20 callbacks.
  - Lo cubre todo, pero con mucha más superficie de la que necesitamos entender.

La capa B del banco de pruebas
([escenarios](../../benchmarks/docs/harness-scenarios.md)) convierte los
requisitos en algo comprobable. **Con la interfaz de byoharness, flores no podría
superarla:**

| Requisito | Escenario que lo exige |
|---|---|
| Streaming | `slow-first-token`, `stream-truncated-recover`, `huge-output`; `ttft_ms` en la trayectoria |
| Errores clasificados | `rate-limit-recover` (429 + Retry-After), `server-error-recover` (503), `auth-error-fail-fast` (401 sin reintentos) |
| Consumo de tokens | coste en la capa C; `tokens` en la trayectoria |
| Modelo por petición | subagentes con un modelo más barato; comparaciones pareadas |

## Decisión

Un término medio entre byoharness y fantasy:

```go
// internal/provider/provider.go
type Provider interface {
    // Stream hace una petición y entrega los eventos según llegan.
    // Un error antes del primer evento es de conexión/HTTP; uno a mitad es un
    // stream cortado. Ambos llegan como *provider.Error.
    Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
}

type Request struct {
    Model      string     // por petición: sin estado mutable
    System     string
    Messages   []Message
    Tools      []ToolDef  // ordenadas por nombre: bytes estables para la caché
    ToolChoice ToolChoice // Auto (por defecto) | None: el paso de resumen (ADR 0003)
    MaxTokens  int
}

type Event struct {
    Kind     EventKind   // Sent | TextDelta | ToolCall | Done
    Bytes    int         // en Sent: tamaño del cuerpo enviado (request_bytes)
    Text     string
    ToolCall *ToolCall   // completa: el adaptador ensambla los fragmentos
    Usage    Usage       // en Done
    Stop     StopReason  // en Done: EndTurn | ToolUse | MaxTokens
}

type Error struct {      // una taxonomía común a todos los proveedores
    Kind       ErrorKind // RateLimit | Overloaded | Auth | BadRequest | Network | Truncated
    Status     int
    RetryAfter time.Duration
    Err        error
}

func (e *Error) Retryable() bool
```

Las decisiones de diseño:

1. **Un único método, `Stream`.** La versión sin streaming es un helper,
   `provider.Collect(stream)`, no un segundo método que todos los adaptadores
   tengan que implementar.
2. **Iterador `iter.Seq2` (Go 1.23+).** Se consume con un `for … range`, no hace
   falta gestionar goroutines ni cerrar canales. Es la misma elección que fantasy.
3. **Los reintentos son un decorador:** `provider.WithRetry(p, policy)`. Se
   escriben una vez para todos los proveedores y se prueban contra los
   escenarios del mock.
4. **Las tool calls se emiten completas.** Ensamblar los argumentos que llegan a
   trozos por streaming es trabajo del adaptador; el bucle del agente no lo ve.
5. **Mensajes con partes tipadas:** `Text`, `ToolCall`, `ToolResult` y, más
   adelante, `Reasoning`, recorridas con un `switch` de tipos. Es más claro que
   un único `Block` con campos opcionales según su `Type`.
6. **Dos adaptadores al empezar:**
   - `anthropic`, para la Messages API.
   - `openai`, para Chat Completions. El mismo adaptador sirve para OpenAI,
     Ollama, LM Studio, vLLM y el endpoint compatible de Cloudflare cambiando
     solo la base URL.
   - La Responses API se añadirá solo si un benchmark demuestra que hace falta
     (por ejemplo, para el razonamiento de los modelos `gpt-6.x`).

## SDK oficial o `net/http` propio: decidido, `net/http`

Esta sección empezó como pregunta abierta. Se cerró el 2026-11-05 tras mirar qué
hay **dentro** del adaptador en cada referencia. La interfaz propia es lo que da
la independencia del proveedor; el SDK o el `net/http` solo deciden qué hay
detrás de ella:

| Proyecto | Dentro del adaptador |
|---|---|
| byoharness (cap. 3) | SDK oficial (`client.Messages.New`, `openai.NewClient`) |
| crush → fantasy | SDK oficiales: `anthropic-sdk-go`, un fork de `openai-go`, `genai` |
| Pi (`pi-ai`) | SDK oficiales: `@anthropic-ai/sdk`, `openai`, `@google/genai` |
| opencode (`packages/ai`) | protocolo propio, un fichero por protocolo, sin SDK de modelos |
| fx | HTTP y SSE propios; habla Chat Completions y Responses, y llega a Claude a través del AI Gateway de Vercel |

**Decisión: `net/http` propio, un paquete por protocolo** (como opencode y fx).
El primero es `anthropic` (Messages API); `openai` (Chat Completions y
compatibles) llega en el hito M1. Los motivos:

- **Es el objetivo educativo:** ver el protocolo real (SSE, `tool_calls`,
  `tool_use`). Ya lo hicimos así en el mock y en el stub.
- **Binario y arranque mínimos.** fx lo hace igual (en Zig) y es el más ligero
  de la tabla de la capa A.
- **Cero dependencias**, y por tanto más portable.
- **Se puede medir:** `perf-startup`, `perf-hello` y el tamaño del binario dirán
  si la decisión fue buena.

A favor de los SDKs: cubren los casos raros del protocolo y se actualizan solos
cuando cambia la API. Aceptamos ese coste: los cambios de protocolo nos tocará
seguirlos a nosotros, y los escenarios de la capa B contra el mock avisarán si
algo se rompe.

## Alternativas descartadas

- **La interfaz de byoharness tal cual.** No supera la capa B (ver la tabla).
- **Usar fantasy como dependencia.** Esconde justo lo que queremos aprender y
  añade mucha superficie.
- **Canales en lugar de iterador.** Obligan a gestionar goroutines y su cierre,
  y es fácil que se queden colgadas si se cancela a mitad.
- **Callbacks (`OnTextDelta`, `OnToolCall`…).** Reparten el control de flujo
  entre muchas funciones; con un iterador el bucle se lee de arriba abajo.

## Cómo se valida

- Los escenarios de la capa B pasan con los dos adaptadores (`anthropic` y
  `openai`) contra el mock.
- Ningún tipo de un SDK o de un protocolo concreto aparece fuera de
  `internal/provider/<adaptador>/`.
- Añadir un proveedor nuevo es un paquete nuevo y una línea en `cmd/flores/main.go`.
