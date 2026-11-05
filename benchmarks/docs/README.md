# Documentación de `benchmarks/`

Aquí hay tres tipos de documento. Conviene no mezclarlos.

| Tipo | Para quién | Dónde vive | Estado |
|---|---|---|---|
| **Referencia técnica** | quien quiera usar, tocar o entender el código del repo | esta carpeta (`benchmarks/docs/`) | escrita |
| **Guía para la web** | quien no sabe nada de agentes y quiere aprender qué es flores y cómo se hizo | la futura web (no está en este repo todavía) | pendiente |
| **Notas internas** | nosotros: tareas pendientes y decisiones por tomar | [`internal/`](internal/) | viva |

La referencia técnica **no explica qué es un agente**. Da por hecho que lees
el [glosario](glossary.md) si algún término te suena raro. La guía de la web sí
partirá de cero.

## Referencia técnica

Para entender el proyecto, léela en este orden:

1. [Glosario](glossary.md): las palabras que usamos, en lenguaje llano.
2. [Metodología](methodology.md): qué medimos, cómo y con qué límites.
3. [Clasificación de pruebas](test-taxonomy.md): qué pruebas hay, cuáles son
   fiables al 100 % y cuáles son ruidosas.
4. [Pipeline y arquitectura](pipeline.md): qué hace cada pieza y qué ejecuta la CI.

Para consultar cuando lo necesites:

- [Métricas](metrics.md): qué significa cada número.
- [Escenarios de la capa B](harness-scenarios.md): cada fallo que simulamos (generado).
- [Contrato headless y trayectorias](trajectory-schema.md): lo que `flores run` debe emitir.
- [Formato de resultados](results-format.md): cómo se guardan y publican.
- [Adaptadores](adapters.md): cómo se conecta cada agente al mock.

## Guía para la web (pendiente)

Se escribirá aparte. Debe cubrir, en este orden: qué es un agente, cómo
funciona su bucle, qué principios seguimos para construir el nuestro, por qué
empezamos por los benchmarks y cómo leer los resultados. No debe copiar la
referencia técnica: la enlaza.
