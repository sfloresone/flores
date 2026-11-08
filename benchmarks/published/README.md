# Resultados publicados

> Fichero generado por `flores-bench publish` / `flores-bench report`. No editar a mano.
> Fuente de verdad: los `latest.json` / `history.jsonl` de cada carpeta.
> Metodología: [`../docs/methodology.md`](../docs/methodology.md).

p50 = mediana; entre corchetes, intervalo de confianza del 95 % de la mediana (bootstrap).

## Comportamiento del harness (capa B): fallos guionizados

### `linux-x86_64`

Commit `7150f1c0af` (dirty) · 2026-11-08T15:31:34Z · 1 ejecución(es) por escenario

Escenarios y su porqué: [`../docs/harness-scenarios.md`](../docs/harness-scenarios.md).

| Escenario | flores<br><sub>0.0.0-dev</sub> | flores (OpenAI)<br><sub>0.0.0-dev</sub> | flores-stub<br><sub>0.0.0-stub</sub> | Pi<br><sub>1.5.0</sub> | Claude Code<br><sub>2.1.318</sub> | Codex<br><sub>0.164.0</sub> | fx<br><sub>0.0.16</sub> |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **Respuesta de texto simple**<br><sub>`basic-text` · basics</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Argumentos de tool con JSON roto**<br><sub>`malformed-tool-args` · tools</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | — | ✅ |
| **Dos tool calls en el mismo turno**<br><sub>`parallel-tools` · tools</sub> | ❌ | ❌ | ❌ | ✅ | ✅ | — | ✅ |
| **Tres turnos de tools encadenados**<br><sub>`tool-chain` · tools</sub> | ❌ | ❌ | ❌ | ✅ | ✅ | — | ✅ |
| **Una tool call y su resultado vuelve al modelo**<br><sub>`tool-roundtrip` · tools</sub> | ❌ | ❌ | ❌ | ✅ | ✅ | — | ✅ |
| **El modelo pide una tool que no existe**<br><sub>`unknown-tool` · tools</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **401: fallar rápido y sin reintentos**<br><sub>`auth-error-fail-fast` · provider-errors</sub> | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| **Sin saldo: fallar rápido y sin reintentos**<br><sub>`out-of-credit-fail-fast` · provider-errors</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **429 transitorio: reintentar respetando Retry-After**<br><sub>`rate-limit-recover` · provider-errors</sub> | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **429 con Retry-After de una hora: fallar en el acto**<br><sub>`retry-after-too-long` · provider-errors</sub> | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **503 transitorio: reintentar**<br><sub>`server-error-recover` · provider-errors</sub> | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Respuesta de 1 MiB**<br><sub>`huge-output` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Primer token a los 8 s**<br><sub>`slow-first-token` · streaming</sub> | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Stream cortado a mitad: reintentar**<br><sub>`stream-truncated-recover` · streaming</sub> | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **El modelo pide tools sin fin**<br><sub>`runaway-tool-loop` · limits</sub> | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Superados / aplicables | **12/15** | **12/15** | **9/15** | **13/15** | **13/15** | **8/11** | **12/15** |

✅ cumple la especificación · ❌ no la cumple · — no aplicable al agente · ⚠️ error del runner

<details><summary>Por qué falla cada caso</summary>

- **flores** · `parallel-tools`: request_contains: esperado flores-11, observado absent; request_contains: esperado flores-22, observado absent <sub>(exit 0, 2 peticiones, 4 ms)</sub>
- **flores** · `tool-chain`: request_contains: esperado flores-101, observado absent; request_contains: esperado flores-102, observado absent; request_contains: esperado flores-103, observado absent <sub>(exit 0, 4 peticiones, 5 ms)</sub>
- **flores** · `tool-roundtrip`: request_contains: esperado flores-42, observado absent <sub>(exit 0, 2 peticiones, 4 ms)</sub>
- **flores (OpenAI)** · `parallel-tools`: request_contains: esperado flores-11, observado absent; request_contains: esperado flores-22, observado absent <sub>(exit 0, 2 peticiones, 4 ms)</sub>
- **flores (OpenAI)** · `tool-chain`: request_contains: esperado flores-101, observado absent; request_contains: esperado flores-102, observado absent; request_contains: esperado flores-103, observado absent <sub>(exit 0, 4 peticiones, 5 ms)</sub>
- **flores (OpenAI)** · `tool-roundtrip`: request_contains: esperado flores-42, observado absent <sub>(exit 0, 2 peticiones, 4 ms)</sub>
- **flores-stub** · `parallel-tools`: request_contains: esperado flores-11, observado absent; request_contains: esperado flores-22, observado absent <sub>(exit 0, 2 peticiones, 5 ms)</sub>
- **flores-stub** · `tool-chain`: request_contains: esperado flores-101, observado absent; request_contains: esperado flores-102, observado absent; request_contains: esperado flores-103, observado absent <sub>(exit 0, 4 peticiones, 5 ms)</sub>
- **flores-stub** · `tool-roundtrip`: request_contains: esperado flores-42, observado absent <sub>(exit 0, 2 peticiones, 4 ms)</sub>
- **flores-stub** · `rate-limit-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after rate limit., observado ; llm_requests: esperado {'min': 3, 'max': 3}, observado 1; retries: esperado {'min': 2, 'max': 2}, observado 0 <sub>(exit 3, 1 peticiones, 3 ms)</sub>
- **flores-stub** · `server-error-recover`: exit: esperado zero, observado 3; stdout_contains: esperado Recovered after server error., observado ; retries: esperado {'min': 1, 'max': 1}, observado 0 <sub>(exit 3, 1 peticiones, 3 ms)</sub>
- **flores-stub** · `stream-truncated-recover`: exit: esperado zero, observado 3; stdout_contains: esperado The quick brown fox jumps over the lazy dog., observado ; retries: esperado {'min': 1}, observado 0 <sub>(exit 3, 1 peticiones, 4 ms)</sub>
- **Pi** · `retry-after-too-long`: llm_requests: esperado {'min': 1, 'max': 1}, observado 4; max_wall_ms: esperado 5000, observado 14353 <sub>(exit 1, 4 peticiones, 14353 ms)</sub>
- **Pi** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 2157 <sub>(exit -9, 2157 peticiones, 45070 ms)</sub>
- **Claude Code** · `auth-error-fail-fast`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'min': 1, 'max': 1}, observado 7; max_wall_ms: esperado 10000, observado 60070 <sub>(exit -9, 7 peticiones, 60070 ms)</sub>
- **Claude Code** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 1588 <sub>(exit -9, 1588 peticiones, 45066 ms)</sub>
- **Codex** · `auth-error-fail-fast`: llm_requests: esperado {'min': 1, 'max': 1}, observado 6 <sub>(exit 1, 6 peticiones, 6267 ms)</sub>
- **Codex** · `retry-after-too-long`: completes: esperado exits before timeout, observado timeout; max_wall_ms: esperado 5000, observado 45008 <sub>(exit -9, 1 peticiones, 45008 ms)</sub>
- **Codex** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 1814 <sub>(exit -9, 1814 peticiones, 45009 ms)</sub>
- **fx** · `out-of-credit-fail-fast`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'min': 1, 'max': 1}, observado 7; max_wall_ms: esperado 5000, observado 60034 <sub>(exit -9, 7 peticiones, 60034 ms)</sub>
- **fx** · `retry-after-too-long`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'min': 1, 'max': 1}, observado 2; max_wall_ms: esperado 5000, observado 45001 <sub>(exit -9, 2 peticiones, 45001 ms)</sub>
- **fx** · `runaway-tool-loop`: completes: esperado exits before timeout, observado timeout; llm_requests: esperado {'max': 50}, observado 2480 <sub>(exit -9, 2480 peticiones, 45049 ms)</sub>

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

Commit `7150f1c0af` (dirty) · 2026-11-08T15:23:01Z · INTEL(R) XEON(R) PLATINUM 8573C · 4 cores · CI · 3×5 runs (+1 warmup)

| Agente | Versión | Tiempo p50 (ms) | Harness p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) | Enviado al modelo (KiB) | System prompt (KiB) | Tools | Peticiones |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| flores-stub | 0.0.0-stub | 3.35 <sub>[3.2–3.6]</sub> | 3.12 | 3.66 | 7.74 | 0.2 | 0.1 | 0 | 1 |
| flores | 0.0.0-dev | 3.37 <sub>[3.3–4.1]</sub> | 3.15 | 3.86 | 7.90 | 0.2 | 0.0 | 0 | 1 |
| flores (OpenAI) | 0.0.0-dev | 3.46 <sub>[3.4–3.6]</sub> | 3.25 | 3.92 | 7.96 | 0.2 | 0.0 | 0 | 1 |
| fx | 0.0.16 | 12.7 <sub>[12.6–12.9]</sub> | 12.0 | 2.69 | 7.03 | 24.9 | 9.1 | 13 | 1 |
| Pi | 1.5.0 | 311.4 <sub>[308.0–319.9]</sub> | 310.9 | 424.1 | 112.8 | 5.8 | 2.3 | 4 | 1 |
| Claude Code | 2.1.318 | 354.4 <sub>[348.7–357.8]</sub> | 353.2 | 399.5 | 197.7 | 44.9 | 5.5 | 12 | 1 |
| Codex | 0.164.0 | 436.6 <sub>[432.3–445.8]</sub> | 434.1 | 418.9 | 154.9 | 54.5 | 27.0 | 11 | 1 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["flores-stub", "flores", "flores (OpenAI)", "fx", "Pi", "Claude Code", "Codex"]
    y-axis "Tiempo p50 (ms)"
    bar [3.35, 3.37, 3.46, 12.69, 311.40, 354.38, 436.62]
```

```mermaid
xychart-beta
    title "Bytes enviados al modelo"
    x-axis ["flores-stub", "flores", "flores (OpenAI)", "fx", "Pi", "Claude Code", "Codex"]
    y-axis "Bytes enviados al modelo"
    bar [238.00, 194.00, 204.00, 25448.00, 5947.00, 45993.00, 55759.00]
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

Commit `7150f1c0af` (dirty) · 2026-11-08T15:22:37Z · INTEL(R) XEON(R) PLATINUM 8573C · 4 cores · CI · 3×30 runs (+5 warmup)

| Agente | Versión | Tiempo p50 (ms) | CPU p50 (ms) | RSS p50 (MiB) |
|---|---|---:|---:|---:|
| fx | 0.0.16 | 0.22 <sub>[0.2–0.2]</sub> | 0.16 | ≤ 2.5 |
| process baseline (true) | - | 0.38 <sub>[0.4–0.4]</sub> | 0.32 | ≤ 2.6 |
| flores-stub | 0.0.0-stub | 1.79 <sub>[1.8–1.8]</sub> | 2.08 | 6.40 |
| flores (OpenAI) | 0.0.0-dev | 1.84 <sub>[1.8–1.9]</sub> | 2.16 | 6.46 |
| flores | 0.0.0-dev | 1.86 <sub>[1.8–1.9]</sub> | 2.18 | 6.46 |
| Claude Code | 2.1.318 | 6.90 <sub>[6.6–7.1]</sub> | 6.68 | 25.3 |
| Codex | 0.164.0 | 35.5 <sub>[34.7–35.8]</sub> | 38.9 | 50.9 |
| Pi | 1.5.0 | 201.7 <sub>[200.2–203.1]</sub> | 261.1 | 102.8 |

```mermaid
xychart-beta
    title "Tiempo p50 (ms)"
    x-axis ["fx", "process baseline (true)", "flores-stub", "flores (OpenAI)", "flores", "Claude Code", "Codex", "Pi"]
    y-axis "Tiempo p50 (ms)"
    bar [0.22, 0.38, 1.79, 1.84, 1.86, 6.90, 35.47, 201.65]
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
