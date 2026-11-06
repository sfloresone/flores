# 0004. Reintentos de las llamadas al modelo

> 🇬🇧 [English](en/0004-retries.md)

- **Estado:** Aceptada
- **Fecha:** 2026-11-06
- **Amplía:** el punto 3 del [ADR 0002](0002-interfaz-provider.md) (el decorador `WithRetry`)

## Contexto

Los proveedores fallan de forma transitoria: un 429 por límite de peticiones, un
503 o un 529 por sobrecarga, o un stream SSE que se corta a mitad. En M0, un
solo fallo de estos terminaba el turno con el código 3. En la capa B, flores
suspendía `rate-limit-recover`, `server-error-recover` y
`stream-truncated-recover`, que los cuatro de referencia pasan.

Antes de escribir los reintentos leímos cómo los hacen (código fijado a un commit):

| | Dónde | Intentos | Espera | `Retry-After` | 401 | Corte a mitad de respuesta |
|---|---|---|---|---|---|---|
| **fx** | El bucle del agente ([`recovery.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/recovery/recovery.zig#L203)); el transporte hace un solo intento | **Sin límite**, hasta Esc ([`request_recovery.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/agent/runtime/request_recovery.zig#L28-L30)) | 1, 2, 4, 8, 16 y 30 s, ×0,8–1 | Solo en segundos y [recortado a 30 s](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/recovery/recovery.zig#L397-L401): si el servidor pide una hora, reintenta a los 30 s | Un reintento si puede refrescar la credencial | Desde cero; el texto parcial se queda en pantalla con "↻ Restarted the answer" |
| **opencode** | El runner de sesión, por paso | 1 + 10 | 2, 4, 8 y luego 10 s, con jitter ([`model-retry.ts`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/model-retry.ts#L22-L36)) | `-ms`, segundos o fecha; sirve de espera mínima, con [tope de 15 min](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/model-retry.ts#L12) | No, salvo `x-should-retry: true` o un plugin | Si aún no había salida, desde cero. Si la había, añade un mensaje: ["Continue from where you left off"](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/session/runner/llm.ts#L37-L38) |
| **crush** (fantasy) | El bucle de fantasy, por paso; el SDK va con `MaxRetries(0)` | [1 + 3](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/retry.go#L113-L119) | 5, 10 y 20 s, sin jitter ni tope | `-ms`, segundos o fecha, pero [si pide 60 s o más lo ignora](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/retry.go#L47-L53) y reintenta antes. Además, [`toHeaderMap`](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/anthropic/error.go#L130-L139) guarda las claves como `Retry-After` y luego busca `retry-after` | Un `OnAuthRefresh` | Desde cero; borra el contenido parcial |
| **Pi** | La sesión (`agent.continue()`); la capa del proveedor está [apagada por defecto](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/utils/provider-retry.ts#L111) | [1 + 3](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/coding-agent/src/core/settings-defaults.ts#L18-L26) | 2, 4 y 8 s, sin jitter | Ignorado por defecto: solo lo lee la capa del proveedor | No | Desde cero; el parcial queda en la sesión, oculto al modelo |

Tiempo total de recuperación en la capa B (Linux, publicado):

| | 503 | Stream cortado | 429 con `Retry-After: 1`, dos veces |
|---|---|---|---|
| fx | 0,29 s | 0,29 s | 2,06 s |
| Codex | 0,55 s | 0,54 s | 2,37 s |
| Claude Code | 0,92 s | 0,92 s | 2,66 s |
| Pi | 2,3 s | 2,3 s | 6,3 s |

Con 429 el mínimo posible son 2 s, porque el servidor pide dos esperas de 1 s.

## Decisión

1. **La política es una función pura.** `provider.RetryPolicy.Delay(err, n)`
   devuelve cuánto esperar antes del reintento `n`, o por qué no reintentar.
   No hace E/S ni lee el reloj, como el `recovery.zig` de fx, así que se prueba
   con una tabla.
2. **Qué se reintenta:** red, 408, 409, 429, 5xx (incluido el 529), un `error`
   SSE de sobrecarga o de límite, y el stream que termina sin `message_stop`.
   - **Un 401 o un 403 no se reintenta nunca**, aunque la respuesta traiga
     `x-should-retry: true`. Con una API key, reintentar no arregla nada.
   - Para el resto, `x-should-retry` (`true` o `false`) manda sobre la
     clasificación.
3. **Cuánto se espera depende de la causa:**
   - **Conexión caída o stream cortado:** el primer reintento sale a los
     ~100 ms, porque basta con una conexión nueva. Después, como el resto.
   - **Sobrecarga o 5xx:** 0,5 s, que se duplica hasta un tope de 30 s, con un
     jitter del 80–100 % para que muchos clientes no reintenten a la vez.
   - **`Retry-After`** (o `retry-after-ms`) se respeta **exactamente** hasta
     60 s. Si el servidor pide más, flores **falla en el acto** con un mensaje
     claro: ni reintenta antes de tiempo (como fx y crush) ni deja colgado al
     usuario.
   - Si la espera no cabe en lo que queda de `--timeout`, falla ya.
4. **6 reintentos por defecto** (7 intentos), configurables con
   `--max-retries`. Con 0 se desactivan.
   - Hay un límite porque en modo headless (CI) nadie está delante para pulsar
     Esc, que es lo único que para a fx.
   - Un reintento no es un paso: no cuenta para `--max-steps` ni para la
     detección de repeticiones ([ADR 0003](0003-bucle-del-agente.md)).
5. **El decorador avisa antes de reintentar.** `provider.WithRetry(p, policy)`
   emite un `EventRetry` y luego espera; la espera se puede cancelar con
   Ctrl-C. Después reenvía **la misma petición**. Al recibir `EventRetry`, el
   bucle descarta el texto y las tool calls parciales. Esto amplía el punto 3
   del ADR 0002: sin la señal, el bucle recibiría el texto dos veces.
6. **El modelo no ve nada de los intentos fallidos.** El mensaje del asistente
   solo entra en el historial cuando el paso termina bien. No se le pide que
   "continúe", como hace opencode.
7. **Todo reintento queda a la vista:**
   - en la trayectoria, el `llm.request` lleva `attempt`, y el `error` lleva
     `retryable`, `attempt` y `retry_in_ms`;
   - en modo texto, un aviso por stderr: `flores: overloaded (HTTP 503) —
     retrying in 0.5s (attempt 2 of 7)`.

   El texto parcial ya escrito en stdout se queda, como en fx y Pi; quien
   necesite una salida limpia usa `--output jsonl` (`final_text`).

## Por qué

- **Un límite, a diferencia de fx.** Sin límite, un modo headless puede quedarse
  colgado indefinidamente. Para una persona en una TUI eso es aceptable; para
  un script, no.
- **Esperas según la causa.** Un corte de red se arregla reconectando, así que
  esperar segundos solo hace perder tiempo. Una sobrecarga necesita aire, y
  reintentar enseguida la empeora.
- **Respetar `Retry-After` o fallar.** El servidor sabe cuándo podrá atendernos.
  Reintentar antes (fx a los 30 s, crush con su espera exponencial, Pi a los
  2 s) gasta peticiones que van a fallar y puede alargar el bloqueo. Esperar
  una hora sin avisar es peor.
- **La misma petición, sin "continúa".** Reenviar los mismos bytes es
  determinista, reaprovecha la caché del prompt y no deja en el historial
  mensajes que el usuario no escribió. Pedir al modelo que continúe ahorra
  tokens de salida en respuestas largas, pero el modelo puede repetir o
  saltarse texto. Se reconsiderará si el banco muestra que importa.

## Alternativas descartadas

- **Reintentar dentro de `Stream`, sin avisar** (el decorador transparente del
  ADR 0002 original). Es imposible hacerlo bien si ya llegaron deltas: el
  consumidor los mostraría dos veces.
- **Reintentar en el bucle del agente** (fx, crush). Funciona, pero la política
  queda atada al bucle. Con el decorador, cualquier llamada al modelo hereda
  los reintentos, como lo harán en el futuro la compactación o los títulos.
- **Esperas fijas sin jitter** (crush, Pi). Si muchos clientes fallan a la vez,
  reintentan a la vez.

## Cómo se valida

- **Tests de tabla** de `Delay`: el 401 nunca, `Retry-After` exacto, más de
  60 s → falla, tope, jitter con un `Rand` fijo.
- **Tests del decorador** con un proveedor falso: corte a mitad y luego éxito,
  cancelación durante la espera, deadline, el consumidor que deja de iterar.
- **Capa B:**
  - `rate-limit-recover`, `server-error-recover` y `stream-truncated-recover`
    pasan a `must_pass`;
  - el escenario nuevo `retry-after-too-long` (429 con `Retry-After: 3600`)
    exige fallar con una sola petición.
- **Objetivos de tiempo:** 503 ≈ 0,5 s, stream cortado ≈ 0,1 s, 429 ≈ 2,0 s.
