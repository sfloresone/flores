# Pipeline y arquitectura

> Referencia técnica. Palabras raras: [glosario](glossary.md).
> Qué pruebas hay y cuáles son fiables: [test-taxonomy.md](test-taxonomy.md).

## Qué hace cada pieza

| Pieza | Qué hace | Qué no debe hacer |
|---|---|---|
| `tools/mockprovider/` (Go) | Imita a los proveedores, contesta según el escenario y apunta lo que recibe | Ejecutar agentes, calcular estadística o publicar informes |
| `tools/flores-stub/` (Go) | Agente mínimo que cumple el contrato. Sirve para probar y calibrar el circuito | Hacerse pasar por el agente real |
| `tools/meter/` (Go) | Proxy entre los agentes y los proveedores reales: guarda las claves, mide tokens y coste, corta el gasto | Escuchar en una interfaz pública o reenviar peticiones de modelos sin precio |
| Harbor (dependencia fijada) | Construir la imagen de cada tarea, instalar el agente, ejecutar el verificador | Ver claves reales (el runner se las quita del entorno) |
| `flores_bench/` (Python) | Lanza los agentes, aísla el entorno, repite, mide y genera los informes | Reimplementar el mock o tener reglas del workflow de GitHub |
| `scenarios/`, `suites/`, `schema/` | Datos: guiones, versiones, límites y contratos. Los leen Go y Python | Esconder lógica de ejecución |
| `.github/workflows/bench.yml` | Decide cuándo se prueba, se mide y se publica | Calcular estadística o definir la metodología |

**¿Por qué Go y Python?** Lo que se ejecuta (mock y stub) está en Go, que es el
lenguaje de flores. La orquestación (lanzar procesos, leer JSON, informes) es más
cómoda en Python. Hacerlo todo en Go complicaría los adaptadores; hacer el mock en
Python repetiría trabajo.

**Limitación:** el medidor usa `os.wait4`, que solo existe en macOS y Linux. En
Windows habría que adaptarlo.

## Recorrido de los datos

```
scenarios/*.json ─▶ mock provider ◀── agente (aislado: sin claves, config y carpeta vacías)
                         │                │
                 peticiones recibidas   trayectoria (si el agente la emite)
                         └──────┬─────────┘
                          flores_bench
                                │
              results/*.json    resultado crudo, con todas las muestras (no se versiona)
                                │  publish
        published/**/latest.json y history.jsonl    resultado compacto (se versiona)
                                │  report
                  published/README.md    informe generado → futura web
```

## Qué ejecuta la CI

Hay cuatro situaciones. Cada una responde a una pregunta distinta.

| # | Cuándo | Qué hace | Niveles | Claves de API |
|---|---|---|---|---|
| 1 | en cada PR | tests de Go y Python, lint, escenarios y esquemas válidos, contrato | N0, N1 | no |
| 2 | en cada PR | pasada corta de rendimiento con el stub, comprobando que cada ejecución fue válida | N1, N2 | no |
| 3 | al fusionar en `main`, a mano o programada | todos los agentes instalados, más repeticiones, publica los resultados | N1, N2 | no |
| 4 | solo a mano | tareas reales con modelo real, con límite de gasto | N3 | solo en este trabajo |

**Dónde estamos hoy:**

- Está el 1 (job `test`, incluida la puerta de la capa B con flores-stub).
- El 2 y el 3 están **fusionados** en un solo job (`perf`): en cada PR corre la
  pasada completa con todos los agentes, no una corta con el stub. Separarlos
  y añadir `schedule` está pendiente ([notas internas](internal/robustness-backlog.md)).
- El 4 está en `.github/workflows/tasks.yml`: valida las tareas (oracle/nop, N1) cuando cambian, y ejecuta campañas con modelos reales solo a mano (`workflow_dispatch`), con presupuesto obligatorio y los secretos únicamente en ese job.
- Los avisos y los fallos de la capa B (comportamiento) se aplican en cualquier
  plataforma; los de tiempo, solo en Linux.

## Principios

1. **Medir el harness sin el modelo.** Son preguntas distintas: "¿cuánto cuesta
   ejecutar el agente?" y "¿resuelve la tarea?". Nunca se mezclan en un número.
2. **Una ejecución solo cuenta si es válida.** Debe salir con código 0 y contener
   la respuesta exacta del guion. Si no, se descarta y se cuenta como fallo.
3. **Aislamiento y cero gasto.** Cada ejecución tiene su config y su carpeta, un
   `HOME` propio y ninguna clave real. Si un adaptador está mal, debe fallar, no
   gastar dinero.
4. **Versiones fijadas.** Los competidores están en `agents.toml`. Si la versión es
   otra, se marca con ⚠.
5. **Primero lo determinista.** Lo que bloquea un cambio no depende de la velocidad
   de la máquina.
6. **Permisos mínimos.** Solo el paso que publica puede escribir en el repo, y solo
   en `main`.
7. **Todo queda registrado.** Cada resultado guarda commit, si había cambios sin
   guardar, CPU, sistema operativo y versiones de las herramientas.
