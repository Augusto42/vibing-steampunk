# PR #10: reconciliação e roteiro de homologação

Atualizado em 2026-10-14. Candidato de integração, ainda não lançado.

A reconciliação une a branch `codex/upstream-integration-2026-09-25` com a
`main` do fork em `d0d3681103dc3a87a705b536f83489c92e9502d1`. O ponto de partida
do PR era `196fa2aa542d04a97b4cc861e28075f0128401d2`. Os 60 arquivos em conflito
foram reconciliados preservando os contratos atuais do upstream e os
comportamentos específicos do fork. O histórico das duas branches foi mantido.

## Comportamentos preservados

| Área | Resultado da reconciliação |
| --- | --- |
| ENHO/XH | Leitura, atualização de objetos existentes, conferência do proprietário CTS e leitura da fonte ativa após escrita. Criação exige metadados explícitos. |
| Enhancements de classe e BAdI | Criação explícita de CLASS/BADI; leitura dos includes gerados de classe e dos metadados de implementação BAdI pelo bridge. |
| INCL e Dynpro | Include como objeto de primeira classe, visão mesclada somente para leitura, busca em enhancements e leitura de Dynpro. |
| Segurança CLI | Flags herdadas pelos subcomandos; políticas do perfil e overrides explícitos chegam ao cliente sem descartar restrições. |
| Existência e transporte | Create/update/upsert recusam consultas inconclusivas. Create recusa objetos existentes. WriteSource exige request explícita para pacote transportável antes da primeira mutação. |
| Sessão ADT | Cookies, CSRF stateful, controle da janela de lock e validação da correlação de transporte permanecem ativos. O marcador de pacote continua restrito ao objeto verificado. |
| Erros e instalação | Falhas lógicas de escrita/ativação conservam os diagnósticos. Instalação exige leitura de verificação. Serviços AMDP/abapGit opcionais não são anunciados como disponíveis sem implantação. |
| Renomeação | A escrita usa a URL de fonte. Falha ao desbloquear ou ativar interrompe o fluxo antes de excluir o objeto original. |
| Portabilidade | SQLite em Go puro, caminhos portáveis, IDs de gravação únicos e fixtures portáveis. |
| Distribuição | Políticas de contribuição, segurança e sincronização revisada; publicação no fork com versão community explícita e assets LICENSE/NOTICE. |

Os serviços compartilhados do bridge são comparados entre `embedded/abap` e
`src/` por teste de paridade. O helper de movimentação é exclusivo do
instalador; o exemplo AMDP mantém os nomes próprios de cada distribuição.
A correção do parsing de chaves JSON dentro de strings foi preservada no
handler APC junto com a compatibilidade de regex do fork.

## Evidência local

Executado no Windows, com Go 1.26.6 e `CGO_ENABLED=0`:

| Verificação | Resultado |
| --- | --- |
| `go build -p 1 ./...` | Passou |
| `go test -p 1 ./... -count=1` | 24 pacotes passaram; 2.156 testes/subcasos passaram; 7 foram ignorados pelas condições próprias da suíte; nenhuma falha |
| `go vet -p 1 ./...` | Passou |
| `go vet -p 1 -tags integration ./...` | Passou; compilação/análise apenas, sem execução de testes SAP |
| `scripts/test-object-expansion-mock.ps1` | Leitura ENHO, leitura Dynpro por WebSocket e escrita/leitura INCL passaram no servidor sintético em loopback |
| `npm audit --omit=dev` | Nenhuma vulnerabilidade reportada |
| Revisão dos arquivos alterados | Sem marcadores de conflito, identificadores de objetos do cliente, chaves privadas ou tokens de provedor encontrados nas verificações por padrões |

Variáveis de conexão SAP/VSP foram removidas apenas do ambiente dos processos
de validação. Não houve conexão, instalação, ativação, escrita ou transporte
em sistema SAP real. Os testes usaram fixtures inventadas. A varredura por
padrões é evidência complementar à revisão; não constitui garantia universal
de ausência de segredos.

A CI do PR verifica Linux e Windows, build, vet, testes e compilação dos
testes com tag de integração. O job Windows também executa o simulador CLI;
há auditoria de dependências e lint consultivo. Consulte o resultado da
revisão exata no [PR #10](https://github.com/Augusto42/vibing-steampunk/pull/10).

## Homologação pendente em ambiente não produtivo

Este roteiro não foi executado em SAP. A execução exige autorização específica
para o sistema de teste. Não usar objetos, código, logs ou dados de clientes.

1. Registrar release SAP/BASIS, banco, suporte ADT e versão do bridge. Preparar
   objetos host, classe e definição BAdI exclusivamente sintéticos. Manter
   credenciais e endpoints fora do repositório e dos relatórios públicos.
2. Compilar e ativar os serviços do bridge da mesma revisão do candidato.
   Conferir a instalação por CLI e por abapGit, incluindo as dependências e
   os serviços opcionais. Em sistemas anteriores ao BASIS 7.55, conferir os
   parsers de regex e uma mensagem JSON contendo chaves dentro de strings.
3. Homologar leitura INCL/ENHO/Dynpro, leitura de fontes geradas de CLASS,
   metadados BAdI, visão mesclada e busca em enhancements. A visão mesclada
   nunca deve ser enviada como fonte de atualização.
4. Homologar criação XH, CLASS e BADI e atualização XH existente em `$TMP`.
   Confirmar subtipo, versão ativa, fonte lida e estado dos objetos em ADT/SE80.
5. Repetir em pacote transportável com request/tarefa explícitas. Conferir
   associação CTS, request versus tarefa retornada pelo lock e recusa de
   transporte incompatível antes da mutação. Não criar request implicitamente.
6. Exercitar read-only, restrição de pacote/operação/transporte, erro de
   sintaxe, ativação recusada, consulta inconclusiva, falha de leitura após
   escrita e falha de desbloqueio. Resultado inconclusivo não autoriza retry
   automático. Confirmar que renomeação falha sem excluir o original.
7. Conferir rollback de criação parcial e limpeza dos objetos sintéticos.
   Registrar somente resultados sanitizados, revisão testada e limitações
   por release/subtipo. Não liberar ou importar transportes nesta homologação.

O candidato está preparado para essa homologação técnica. Merge, publicação
de binário e uso produtivo continuam pendentes de homologação e revisão.
