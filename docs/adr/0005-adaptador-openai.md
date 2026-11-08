# 0005. Adaptador OpenAI (Chat Completions)

- **Estado:** Aceptada
- **Fecha:** 2026-11-08
- **Desarrolla:** el punto 6 del [ADR 0002](0002-interfaz-provider.md) (dos
  adaptadores: `anthropic` y `openai`)

> La versión en inglés se escribirá junto con el resto de traducciones pendientes.

## Contexto

Hasta M1a, flores solo hablaba la Messages API de Anthropic. El ADR 0002 fijó
un segundo adaptador para **Chat Completions**. Es el protocolo que entienden
OpenAI y casi todos los servidores "compatibles": Ollama, LM Studio, vLLM,
OpenRouter, Groq, DeepSeek, el gateway de Cloudflare… Con un solo adaptador,
cambiando la base URL, flores llega a todos ellos.

Antes de escribirlo leímos cómo lo hacen las cuatro referencias, con el código
fijado a un commit: fx
[`fa799f4`](https://github.com/vercel-labs/fx/tree/fa799f45866d7c4148f175e86e868cb535435e1e),
opencode
[`217cc81`](https://github.com/anomalyco/opencode/tree/217cc811de9b0c1594924e89e48705871f1cacea),
fantasy
[`ec25857`](https://github.com/charmbracelet/fantasy/tree/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd)
(la librería que usa crush) y Pi
[`20d86c3`](https://github.com/earendil-works/pi/tree/20d86c3657e90e857309d908b13adcf7fb8d9611).

**Primer hallazgo: ninguno usa Chat Completions para api.openai.com por defecto.**
- opencode
  ([`providers/openai.ts`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/providers/openai.ts#L131-L134)),
  crush
  ([`openai.go`](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/openai/openai.go#L191))
  y Pi
  ([`providers/openai.ts`](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/providers/openai.ts#L9))
  usan la Responses API.
- fx solo usa Chat Completions con proveedores que configura el usuario.

Chat Completions es, en la práctica, **el protocolo de los compatibles**. Eso
confirma el ADR 0002: Chat Completions primero, y Responses solo si un
benchmark de la capa C demuestra que hace falta.

| | fx | opencode | crush (fantasy) | Pi |
|---|---|---|---|---|
| Rol del system | `system` | `system` | `system` | `developer` en modelos de razonamiento |
| Límite de salida | Siempre [`max_tokens`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/gateway/chat_completions_protocol.zig#L585), que los modelos de razonamiento de OpenAI rechazan | `max_completion_tokens`, o [`max_tokens` según el host](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/protocols/openai-chat.ts#L862-L863) | `max_tokens`, o `max_completion_tokens` en modelos de razonamiento | [`max_completion_tokens`, o `max_tokens` según el host](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/api/openai-completions.ts#L834-L841) |
| Fin del stream | [Exige `[DONE]`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/gateway/chat_completions_protocol.zig#L1029): una respuesta completa sin él se descarta | Exige `finish_reason` | Lo infiere si falta | Exige `finish_reason` y tolera que falte `[DONE]` |
| Llamadas sin `index`, o todas con `index` 0 | Error | [Las separa por `id`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/protocols/openai-chat.ts#L1166-L1173) | [Las fusiona](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/openai/language_model.go#L540): concatena los argumentos | [Las fusiona](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/api/openai-completions.ts#L490) |
| `finish_reason: stop` con tool calls | Error | [Se trata como tool calls](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/protocols/openai-chat.ts#L1296-L1302) | [Solo se corrige si está vacío](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/openai/language_model.go#L655-L659): en streaming las llamadas **no se ejecutan** | — |
| Tokens cacheados | No se leen | Aparte, sin restarlos del input | [Se restan del input](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/openai/language_model_hooks.go#L245-L246) | [Se restan del input](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/api/openai-completions.ts#L1515) |
| `insufficient_quota` (429) | [Por el `code` del cuerpo](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/gateway/client.zig#L3868-L3873); no reintenta | Por el código; no reintenta | [**Se reintenta**](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/errors.go#L81-L100): basta el 429 | [Regex sobre el mensaje](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/utils/retry.ts#L18-L20) en la capa del agente; la [capa del proveedor](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/utils/provider-retry.ts#L25-L37), apagada por defecto, sí reintenta |
| Error a mitad del stream | Termina sin reintentar | Se clasifica | Se reintenta según `type`/`code` | Regex sobre el mensaje |
| `Retry-After` | Solo en segundos, con tope de 30 s | `-ms`, segundos o fecha | `-ms`, segundos o fecha, pero [el mapa de cabeceras está roto](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/openai/error.go#L119) y nunca la encuentra | `-ms`, segundos o fecha |
| `refusal` | Falla el turno | Se muestra como texto | Se ignora | Se ignora |
| Ajustes por proveedor | Casi ninguno | ~7 flags y detección por URL | Hooks de `openaicompat` | ~25 flags |

**`out-of-credit-fail-fast`** es un escenario nuevo de la capa B: OpenAI responde
429 `insufficient_quota`, y Anthropic 400 "credit balance is too low". Medido en
local (macOS):

| | Resultado | Peticiones | Tiempo |
|---|---|---|---|
| fx 0.0.16 | ❌ | 7, todas 429 | 60 s, hasta que el runner lo mata |
| Pi 1.5.0 | ✅ | 1 | 205 ms |
| Claude Code 2.1.319 | ✅ | 1 | 191 ms |
| Codex 0.164.0 | ✅ | 1 | 91 ms |
| flores, con los dos protocolos | ✅ | 1 | 8–9 ms |

Sobre fx: el código de `main` (`fa799f4`) ya lee `insufficient_quota`. La versión
fijada en el banco, 0.0.16, todavía no lo hace, o no en este camino.

## Decisión

1. **Un paquete `internal/provider/openai`**, con `net/http` y sin SDK, como
   `anthropic`.
   - La base URL incluye la versión (`https://api.openai.com/v1`,
     `http://localhost:11434/v1`), y el cliente añade `/chat/completions`.
   - El lector SSE y la lectura de errores (`ReadSSE`, `KindForStatus`,
     `RetryAfter`, `ShouldRetry`) pasan a `internal/provider` y los comparten
     los dos adaptadores.
2. **Sin flags por proveedor.** La lista de Pi y opencode existe porque soportan
   decenas de hosts. flores la evita así:
   - **El límite de salida no se envía si vale 0**, que es el valor por defecto.
     Si se fija, va como `max_completion_tokens`, el campo actual de OpenAI. Los
     compatibles que solo conocen `max_tokens` no ven nunca un campo que no
     entienden.
   - **El system va con el rol `system`**, que entiende todo el mundo. OpenAI
     también lo acepta en los modelos de razonamiento.
   - **Las tools van sin `strict`.**

   Un flag se añadirá cuando un proveedor real lo exija, con un test que lo
   demuestre.
3. **Un stream tolerante, pero que no inventa:**
   - Se acepta un final sin `[DONE]` si ya llegó un `finish_reason`: la
     respuesta está completa. Sin `finish_reason`, el stream está truncado y se
     reintenta (ADR 0004).
   - Las llamadas se agrupan por `index`. Si en un índice ocupado llega otro
     `id`, es una llamada nueva; un delta sin `index` continúa la última.
   - `stop` con tool calls es `ToolUse`: lo que cuenta son las llamadas.
   - Una `refusal` se muestra como texto, porque es la respuesta del modelo.
   - El razonamiento (`reasoning_content`) se ignora por ahora. La parte
     `Reasoning` llegará en su propio hito (ADR 0002, punto 5).
4. **Los tokens significan lo mismo en todos los proveedores.**
   `Usage.InputTokens` es el input **no cacheado**, como en la Messages API. En
   Chat Completions, `prompt_tokens` incluye los cacheados, así que el adaptador
   los resta y los pone en `CacheReadTokens`. Sin esto, el coste de la capa C
   contaría dos veces los tokens cacheados.
5. **"Sin saldo" es su propio error: `ErrQuota`.**
   - Se detecta por el `code` o el `type` (`insufficient_quota`), nunca por el
     estado HTTP. En OpenAI es un 429, como "demasiadas peticiones".
   - Un 402 también es `ErrQuota`.
   - En Anthropic, un 400 cuyo mensaje dice "credit balance". Ya no se
     reintentaba; ahora la CLI lo explica.
   - Como `ErrAuth`, **no se reintenta nunca**, aunque `x-should-retry` diga lo
     contrario.
6. **La CLI elige el proveedor con `--provider`** (o `FLORES_PROVIDER`). La clave
   se busca **después** de saber el proveedor: `FLORES_API_KEY`, y luego
   `ANTHROPIC_API_KEY` u `OPENAI_API_KEY`. Así nunca se envía la clave de un
   proveedor a otro.

## Por qué

- **Es lo que hace la mayoría, sin sus fallos.** Cada punto de la decisión
  corresponde a un bug o una diferencia de la tabla, y tiene un test en
  `openai_test.go`.
- **Menos superficie.** Sin flags, el adaptador cabe en dos ficheros que se leen
  de arriba abajo, y el comportamiento es el mismo con OpenAI y con Ollama.
- **Mismos números para los dos protocolos.** El banco mide `flores` y
  `flores-openai` (el mismo binario) con la misma lista `must_pass`. Si un
  adaptador se queda atrás, la CI falla.

## Alternativas descartadas

- **Responses API primero**, como opencode, crush y Pi con OpenAI. La hablan
  OpenAI y pocos más, así que dejaría fuera a los compatibles. Se añadirá si la
  capa C demuestra que la necesitamos, por ejemplo para el razonamiento de los
  modelos `gpt-6.x`.
- **El SDK de OpenAI.** Descartado ya en el ADR 0002. Además, fantasy enseña el
  riesgo: su fork del SDK esconde el `[DONE]` y los errores del stream.
- **Exigir `[DONE]`, como fx.** Tirar una respuesta completa porque falta la
  última línea obliga a repetir la petición y a pagarla dos veces.
- **Detectar la cuota por el mensaje**, como Pi. Es frágil: palabras como
  "billing" o "timeout" aparecen en errores que no tienen nada que ver.

## Cómo se valida

- `openai_test.go` cubre:
  - el cuerpo exacto de la petición;
  - los argumentos troceados;
  - las llamadas paralelas con `index` 0;
  - `stop` con llamadas;
  - el `usage` con cacheados;
  - el final sin `[DONE]` y el final sin `finish_reason`;
  - los errores en el stream;
  - los errores HTTP, incluido `insufficient_quota` con `x-should-retry: true`.
- En la capa B, `flores` y `flores-openai` pasan los mismos 12 escenarios de 15
  (los tres que faltan necesitan tools: M2).
- `out-of-credit-fail-fast` con una sola petición, en los dos protocolos.
