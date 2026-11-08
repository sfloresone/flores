# Decisiones de arquitectura (ADR)

Cada fichero recoge **una** decisión: el contexto, lo que se decidió, por qué y
qué se descartó. Sirven para que dentro de seis meses (o alguien nuevo) entienda
por qué flores es como es, sin tener que reconstruir la conversación.

| Estado | Significado |
|---|---|
| **Propuesta** | Escrita y razonada, pendiente de confirmar. Puede cambiar. |
| **Aceptada** | Se aplica al código. Cambiarla requiere un ADR nuevo que la sustituya. |
| **Sustituida** | Ya no se aplica; enlaza a la que la reemplaza. |

| # | Decisión | Estado |
|---|---|---|
| [0001](0001-estructura-y-estilo-go.md) | Estructura del proyecto y estilo de Go: claridad antes que idioma | Propuesta |
| [0002](0002-interfaz-provider.md) | Interfaz del proveedor de modelos | Aceptada |
| [0003](0003-bucle-del-agente.md) | El bucle del agente: turnos, pasos y frenos | Aceptada |
| [0004](0004-reintentos.md) | Reintentos de las llamadas al modelo | Aceptada |
| [0005](0005-adaptador-openai.md) | Adaptador OpenAI (Chat Completions) | Aceptada |

Plantilla: copia un ADR existente y mantén las secciones *Contexto*, *Decisión*,
*Por qué*, *Alternativas descartadas* y *Cómo se valida*.
