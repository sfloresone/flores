# Adaptadores: cómo se conecta cada agente al mock

Cada adaptador (`flores_bench/adapters/agents.py`) solo responde a una
pregunta: con qué comando, entorno y ficheros de configuración hace este agente
la tarea X contra el proveedor Y. La medición la hacen los runners, con el
mismo código para todos.

Para todos los agentes:

- Directorio de configuración **nuevo y vacío en cada ejecución**, y workspace vacío.
- Entorno saneado: sin claves reales (`SECRET_ENV`) y sin las variables que
  delatan que corre dentro de otro agente (`CLAUDECODE`, …).
- `stdin` = `/dev/null`. Codex, por ejemplo, se queda esperando stdin si no se cierra.
- Un ID de modelo real que el agente conoce (`suites/perf/agents.toml`).

| Agente | Versión ref. | Protocolo | Cómo se apunta al mock | Comando |
|---|---|---|---|---|
| flores-stub | — | OpenAI Chat | `FLORES_BASE_URL=<mock>/v1` | `flores-stub run --prompt …` |
| Pi | 1.5.0 | OpenAI Responses | `PI_CODING_AGENT_DIR` con `models.json` que solo cambia `baseUrl` del proveedor `openai`; `PI_OFFLINE=1` | `pi -p --model openai/gpt-6.1-sol --no-session …` |
| Claude Code | 2.1.318 | Anthropic Messages | `CLAUDE_CONFIG_DIR`, `ANTHROPIC_BASE_URL=<mock>`, `ANTHROPIC_API_KEY=mock`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` | `claude -p --model claude-sonnet-5-5 …` |
| Codex | 0.164.0 | OpenAI Responses | `CODEX_HOME` con `config.toml`: `model_provider = "mock"`, `wire_api = "responses"` | `codex exec --skip-git-repo-check …` |
| fx | 0.0.16 | OpenAI Chat | `HOME` aislado con `.fx/settings.json`: conexión `openai-chat-completions` y `FX_PROVIDER=mock` | `fx ask --no-save …` |

## Cosas que descubrimos al conectarlos (y por qué importan)

1. **fx usó la suscripción del usuario en vez del mock.** La documentación
   actual de fx describe `FX_HOME`, pero la 0.0.16 no lo soporta: leía
   `~/.fx`, encontraba el login de ChatGPT/Codex y respondía "Hello, world!"
   desde OpenAI (unos 5 s) en lugar de "Hello! Nice to meet you." desde el mock
   (unos 20 ms). Lecciones:
   - **Comprobar siempre la respuesta.** `perf-hello` exige el texto exacto del
     guion; si no aparece, la ejecución no cuenta.
   - **Aislar `HOME`**, no solo la variable que diga la documentación.
2. **Claude Code cambia el system prompt según el modelo.** Con un ID que no
   conoce avisa (`unrecognized_model`) y usa otro prompt (5,8 KB frente a
   8,7 KB). Por eso usamos IDs reales.
3. **El número de tools depende del modelo.** Con `gpt-6.1-sol`, que admite
   *tool search*, Pi y Codex no declaran tools en la primera petición; con
   `gpt-5.4-mini`, Pi declara 4 (`read`, `bash`, `edit`, `write`). La métrica
   comparable es `request_bytes`.
4. **`--version` no es representativo.** Claude Code tarda unos 9 ms en
   `--version` y unos 167 ms en un turno completo.

## Ver exactamente qué envía cada agente

```sh
.bin/mockprovider --port 53000 --dump-bodies /tmp/dump &
# lanza el agente apuntando a http://127.0.0.1:53000
ls /tmp/dump   # 000001.json, ... : el cuerpo exacto de cada petición
```

Es la forma más rápida de estudiar el system prompt, las definiciones de tools
y la gestión del historial de cada harness. Material de aprendizaje directo
para diseñar los de flores.

## Añadir un agente

1. Crear una subclase de `Adapter` en `adapters/agents.py` con `name`,
   `binary`, `version_args` y `hello()`.
2. Registrarla en `ADAPTERS` y añadirla a `suites/perf/agents.toml` (modelo y,
   si se instala por npm, `npm` y `version`).
3. `uv run flores-bench perf hello --quick --agents <nombre>` y comprobar que
   `info.unknown_paths` está vacío y que no hay `failures`.
