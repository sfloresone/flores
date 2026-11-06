# Resultados publicados

> Fichero generado por `flores-bench publish` / `flores-bench report`. No editar a mano.
> Fuente de verdad: los `latest.json` / `history.jsonl` de cada carpeta.
> Metodología: [`../docs/methodology.md`](../docs/methodology.md).

p50 = mediana; entre corchetes, intervalo de confianza del 95 % de la mediana (bootstrap).

## Comportamiento del harness (capa B): fallos guionizados

### `linux-x86_64`

Commit `029dc1ff42` (dirty) · 2026-11-06T10:31:20Z · 1 ejecución(es) por escenario

Escenarios y su porqué: [`../docs/harness-scenarios.md`](../docs/harness-scenarios.md).

| Escenario | flores-stub<br><sub>0.0.0-stub</sub> | Pi<br><sub>1.5.0</sub> | Claude Code<br><sub>2.1.318</sub> | Codex<br><sub>0.164.0</sub> | fx<br><sub>0.0.16</sub> |
|---|:---:|:---:|:---:|:---:|:---:|
| **Respuesta de texto simple**<br><sub>`basic-text` · basics</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Argumentos de tool con JSON roto**<br><sub>`malformed-tool-args` · tools</sub> | ✅ | ✅ | ✅ | — | ✅ |
| **Dos tool calls en el mismo turno**<br><sub>`parallel-tools` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **Tres turnos de tools encadenados**<br><sub>`tool-chain` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **Una tool call y su resultado vuelve al modelo**<br><sub>`tool-roundtrip` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **El modelo pide una tool que no existe**<br><sub>`unknown-tool` · tools</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **401: fallar rápido y sin reintentos**<br><sub>`auth-error-fail-fast` · provider-errors</sub> | ✅ | ✅ | ❌ | ❌ | ✅ |
| **429 transitorio: reintentar respetando Retry-After**<br><sub>`rate-limit-recover` · provider-errors</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **503 transitorio: reintentar**<br><sub>`server-error-recover` · provider-errors</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Respuesta de 1 MiB**<br><sub>`huge-output` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Primer token a los 8 s**<br><sub>`slow-first-token` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Stream cortado a mitad: reintentar**<br><sub>`stream-truncated-recover` · streaming</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **El modelo pide tools sin fin**<br><sub>`runaway-tool-loop` · limits</sub> | ✅ | ❌ | ❌ | ❌ | ❌ |
| Superados / aplicables | **7/13** | **12/13** | **11/13** | **7/9** | **12/13** |

✅ cumple la especificación · ❌ no la cumple · — no aplicable al agente · ⚠️ error del runner

<details><summary>Por qué falla cada caso</summary>

- **flores-stub** · `parallel-tools`: request_contains: esperado flores-11, observado absent; request_contains: esperado flores-22, observado absent <sub>(exit 0, 2 peticiones, 6 ms)</sub>
- **flores-stub** · `tool-chain`: request_contains: esperado flores-101, observado absent; request_contains: esperado flores-102, observado absent; request_contains: esperado flores-103, observado absent <sub>(exit 0, 4 peticiones, 6 ms)</sub>
- **flores-stub** · `tool-roundtrip`: request_contains: esperado flores-42, observado absent <sub>(exit 0, 2 peticiones, 5 ms)</sub>
- **flores-stub** · `rate-limit-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after rate limit., observado ; llm_requests: esperado {'min': 3, 'max': 3}, observado 1; retries: esperado {'min': 2, 'max': 2}, observado 0 <sub>(exit 3, 1 peticiones, 5 ms)</sub>
- **flores-stub** · `server-error-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after server error., observado ; retries: esperado {'min': 1, 'max': 1}, observado 0 <sub>(exit 3, 1 peticiones, 5 ms)</sub>
- **flores-stub** · `stream-truncated-recover`: exit: esperado zero, observado 3; stdout_contains: esperado The quick brown fox jumps over the lazy dog., observado ; retries: esperado {'min': 1}, observado 0 <sub>(exit 3, 1 peticiones, 5 ms)</sub>
- **Pi** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 1931 <sub>(exit -9, 1931 peticiones, 45080 ms)</sub>
- **Claude Code** · `auth-error-fail-fast`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'min': 1, 'max': 1}, observado 7; max_wall_ms: esperado 10000, observado 60058 <sub>(exit -9, 7 peticiones, 60058 ms)</sub>
- **Claude Code** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 1363 <sub>(exit -9, 1363 peticiones, 45072 ms)</sub>
- **Codex** · `auth-error-fail-fast`: llm_requests: esperado {'min': 1, 'max': 1}, observado 6 <sub>(exit 1, 6 peticiones, 6412 ms)</sub>
- **Codex** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 1679 <sub>(exit -9, 1679 peticiones, 45046 ms)</sub>
- **fx** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 2278 <sub>(exit -9, 2278 peticiones, 45029 ms)</sub>

</details>

### `darwin-arm64`

Commit `65006b335b` (dirty) · 2026-11-05T14:13:36Z · 1 ejecución(es) por escenario

Escenarios y su porqué: [`../docs/harness-scenarios.md`](../docs/harness-scenarios.md).

| Escenario | flores-stub<br><sub>0.0.0-stub</sub> | Pi<br><sub>1.5.0</sub> | Claude Code<br><sub>2.1.318</sub> | Codex<br><sub>0.164.0</sub> | fx<br><sub>0.0.16</sub> |
|---|:---:|:---:|:---:|:---:|:---:|
| **Respuesta de texto simple**<br><sub>`basic-text` · basics</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Argumentos de tool con JSON roto**<br><sub>`malformed-tool-args` · tools</sub> | ✅ | ✅ | ✅ | — | ✅ |
| **Dos tool calls en el mismo turno**<br><sub>`parallel-tools` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **Tres turnos de tools encadenados**<br><sub>`tool-chain` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **Una tool call y su resultado vuelve al modelo**<br><sub>`tool-roundtrip` · tools</sub> | ❌ | ✅ | ✅ | — | ✅ |
| **El modelo pide una tool que no existe**<br><sub>`unknown-tool` · tools</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **401: fallar rápido y sin reintentos**<br><sub>`auth-error-fail-fast` · provider-errors</sub> | ✅ | ✅ | ❌ | ❌ | ✅ |
| **429 transitorio: reintentar respetando Retry-After**<br><sub>`rate-limit-recover` · provider-errors</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **503 transitorio: reintentar**<br><sub>`server-error-recover` · provider-errors</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Respuesta de 1 MiB**<br><sub>`huge-output` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Primer token a los 8 s**<br><sub>`slow-first-token` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Stream cortado a mitad: reintentar**<br><sub>`stream-truncated-recover` · streaming</sub> | ❌ | ✅ | ✅ | ✅ | ✅ |
| **El modelo pide tools sin fin**<br><sub>`runaway-tool-loop` · limits</sub> | ✅ | ❌ | ❌ | ❌ | ❌ |
| Superados / aplicables | **7/13** | **12/13** | **11/13** | **7/9** | **12/13** |

✅ cumple la especificación · ❌ no la cumple · — no aplicable al agente · ⚠️ error del runner

<details><summary>Por qué falla cada caso</summary>

- **flores-stub** · `parallel-tools`: request_contains: esperado flores-11, observado absent; request_contains: esperado flores-22, observado absent <sub>(exit 0, 2 peticiones, 7 ms)</sub>
- **flores-stub** · `tool-chain`: request_contains: esperado flores-101, observado absent; request_contains: esperado flores-102, observado absent; request_contains: esperado flores-103, observado absent <sub>(exit 0, 4 peticiones, 8 ms)</sub>
- **flores-stub** · `tool-roundtrip`: request_contains: esperado flores-42, observado absent <sub>(exit 0, 2 peticiones, 6 ms)</sub>
- **flores-stub** · `rate-limit-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after rate limit., observado ; llm_requests: esperado {'min': 3, 'max': 3}, observado 1; retries: esperado {'min': 2, 'max': 2}, observado 0 <sub>(exit 3, 1 peticiones, 6 ms)</sub>
- **flores-stub** · `server-error-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after server error., observado ; retries: esperado {'min': 1, 'max': 1}, observado 0 <sub>(exit 3, 1 peticiones, 6 ms)</sub>
- **flores-stub** · `stream-truncated-recover`: exit: esperado zero, observado 3; stdout_contains: esperado The quick brown fox jumps over the lazy dog., observado ; retries: esperado {'min': 1}, observado 0 <sub>(exit 3, 1 peticiones, 11 ms)</sub>
- **Pi** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 3485 <sub>(exit -9, 3485 peticiones, 45007 ms)</sub>
- **Claude Code** · `auth-error-fail-fast`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'min': 1, 'max': 1}, observado 7; max_wall_ms: esperado 10000, observado 60010 <sub>(exit -9, 7 peticiones, 60010 ms)</sub>
- **Claude Code** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 3072 <sub>(exit -9, 3072 peticiones, 45006 ms)</sub>
- **Codex** · `auth-error-fail-fast`: llm_requests: esperado {'min': 1, 'max': 1}, observado 6 <sub>(exit 1, 6 peticiones, 6479 ms)</sub>
- **Codex** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 7217 <sub>(exit -9, 7217 peticiones, 45005 ms)</sub>
- **fx** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 2822 <sub>(exit -9, 2822 peticiones, 45001 ms)</sub>

</details>

## Overhead del harness: un "hello world" contra el mock provider

### `linux-x86_64` (referencia)

Commit `029dc1ff42` (dirty) · 2026-11-06T10:25:53Z · AMD EPYC 7763 64-Core Processor · 4 cores · CI · 3×5 runs (+1 warmup)

| Agente | Versión | Tiempo p50 (ms) | Harness p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) | Enviado al modelo (KiB) | System prompt (KiB) | Tools | Peticiones |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| flores-stub | 0.0.0-stub | 4.33 <sub>[4.3–5.0]</sub> | 4.02 | 4.76 | 7.61 | 0.2 | 0.1 | 0 | 1 |
| fx | 0.0.16 | 13.7 <sub>[13.6–13.8]</sub> | 12.8 | 3.71 | 7.04 | 24.9 | 9.1 | 13 | 1 |
| Pi | 1.5.0 | 388.1 <sub>[384.1–389.8]</sub> | 387.4 | 516.3 | 112.5 | 5.8 | 2.3 | 4 | 1 |
| Claude Code | 2.1.318 | 391.5 <sub>[387.5–400.7]</sub> | 389.9 | 459.4 | 197.9 | 44.9 | 5.5 | 12 | 1 |
| Codex | 0.164.0 | 411.7 <sub>[393.4–433.6]</sub> | 409.3 | 541.5 | 154.2 | 54.5 | 27.0 | 11 | 1 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["flores-stub", "fx", "Pi", "Claude Code", "Codex"]
    y-axis "Tiempo p50 (ms)"
    bar [4.33, 13.68, 388.10, 391.51, 411.65]
```

```mermaid
xychart-beta
    title "Bytes enviados al modelo"
    x-axis ["flores-stub", "fx", "Pi", "Claude Code", "Codex"]
    y-axis "Bytes enviados al modelo"
    bar [238.00, 25448.00, 5947.00, 45993.00, 55759.00]
```

### `darwin-arm64`

Commit `65006b335b` (dirty) · 2026-11-05T14:14:17Z · Apple M5 · 10 cores · local · 3×5 runs (+1 warmup)

| Agente | Versión | Tiempo p50 (ms) | Harness p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) | Enviado al modelo (KiB) | System prompt (KiB) | Tools | Peticiones |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| flores-stub | 0.0.0-stub | 5.72 <sub>[5.2–5.9]</sub> | 5.66 | 3.46 | 11.6 | 0.2 | 0.1 | 0 | 1 |
| fx | 0.0.16 | 19.6 <sub>[18.8–21.9]</sub> | 19.4 | 5.91 | 4.86 | 25.0 | 9.3 | 13 | 1 |
| Claude Code | 2.1.318 | 163.4 <sub>[158.0–169.9]</sub> | 163.2 | 190.6 | 193.6 | 45.1 | 5.6 | 12 | 1 |
| Pi | 1.5.0 | 168.3 <sub>[167.5–169.6]</sub> | 168.1 | 196.3 | 113.8 | 5.9 | 2.4 | 4 | 1 |
| Codex | 0.164.0 | 388.1 <sub>[365.0–407.0]</sub> | 387.7 | 142.0 | 101.6 | 54.6 | 27.1 | 11 | 1 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["flores-stub", "fx", "Claude Code", "Pi", "Codex"]
    y-axis "Tiempo p50 (ms)"
    bar [5.72, 19.60, 163.41, 168.32, 388.12]
```

```mermaid
xychart-beta
    title "Bytes enviados al modelo"
    x-axis ["flores-stub", "fx", "Claude Code", "Pi", "Codex"]
    y-axis "Bytes enviados al modelo"
    bar [238.00, 25625.00, 46132.00, 6017.00, 55921.00]
```

## Arranque (`<agent> --version`)

### `linux-x86_64` (referencia)

Commit `029dc1ff42` (dirty) · 2026-11-06T10:25:30Z · AMD EPYC 7763 64-Core Processor · 4 cores · CI · 3×30 runs (+5 warmup)

| Agente | Versión | Tiempo p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) |
|---|---|---:|---:|---:|
| fx | 0.0.16 | 0.40 <sub>[0.4–0.4]</sub> | 0.32 | ≤ 2.5 |
| process baseline (true) | - | 0.59 <sub>[0.6–0.6]</sub> | 0.50 | ≤ 2.6 |
| flores-stub | 0.0.0-stub | 2.51 <sub>[2.5–2.5]</sub> | 2.94 | 6.18 |
| Claude Code | 2.1.318 | 10.4 <sub>[10.3–10.7]</sub> | 10.2 | 28.9 |
| Codex | 0.164.0 | 44.3 <sub>[44.2–44.7]</sub> | 48.6 | 50.9 |
| Pi | 1.5.0 | 241.6 <sub>[240.8–242.3]</sub> | 309.5 | 102.2 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["fx", "process baseline (true)", "flores-stub", "Claude Code", "Codex", "Pi"]
    y-axis "Tiempo p50 (ms)"
    bar [0.40, 0.59, 2.51, 10.42, 44.30, 241.57]
```

### `darwin-arm64`

Commit `65006b335b` (dirty) · 2026-11-05T14:14:04Z · Apple M5 · 10 cores · local · 3×30 runs (+5 warmup)

| Agente | Versión | Tiempo p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) |
|---|---|---:|---:|---:|
| process baseline (true) | - | 0.91 <sub>[0.9–0.9]</sub> | 0.55 | 1.14 |
| fx | 0.0.16 | 2.07 <sub>[2.1–2.1]</sub> | 1.03 | 2.11 |
| flores-stub | 0.0.0-stub | 3.13 <sub>[3.1–3.2]</sub> | 2.15 | 10.0 |
| Codex | 0.164.0 | 5.91 <sub>[5.8–6.0]</sub> | 5.22 | 19.0 |
| Claude Code | 2.1.318 | 8.54 <sub>[8.4–8.6]</sub> | 6.35 | 23.2 |
| Pi | 1.5.0 | 123.4 <sub>[122.8–123.7]</sub> | 129.9 | 103.6 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["process baseline (true)", "fx", "flores-stub", "Codex", "Claude Code", "Pi"]
    y-axis "Tiempo p50 (ms)"
    bar [0.91, 2.07, 3.13, 5.91, 8.54, 123.41]
```
