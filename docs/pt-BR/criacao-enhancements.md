# Criação de enhancements no VSP

Este é o guia em português para criação de implementações do Enhancement
Framework com a distribuição Augusto42 do VSP. O recurso está disponível a
partir da versão `v2.40.0-augusto.1`.

Os binários para Windows, Linux e macOS, junto com os checksums, estão na
[release v2.40.0-augusto.1](https://github.com/Augusto42/vibing-steampunk/releases/tag/v2.40.0-augusto.1).

O VSP cria três tipos de ENHO:

| Comando | Tipo SAP | Resultado |
|---|---|---|
| `xh` | `HOOK_IMPL` | Plug-in de código-fonte em um ponto de enhancement exato |
| `class` | `CLASENH` | Class enhancement vazio ou com um novo método |
| `badi` | `BADI_IMPL` | Implementação de BAdI ligada a uma classe existente |

Para detalhes de todos os parâmetros e do contrato MCP, consulte também o
[guia técnico completo em inglês](../enhancement-creation.md).

## Antes de começar

Use primeiro um sistema de desenvolvimento ou sandbox e objetos sintéticos.
Nunca teste criação diretamente em produção.

1. Instale `v2.40.0-augusto.1` ou uma versão mais recente.
2. Configure o perfil do sistema no `.vsp.json` ou pelas variáveis de ambiente.
3. Instale ou atualize o bridge ZADT_VSP usando o mesmo binário.
4. Confirme as autorizações de desenvolvimento e, se necessário, de CTS.

```powershell
vsp --version
vsp -s dev system info
vsp -s dev install zadt-vsp --package '$ZADT_VSP'
vsp -s dev enhancement create --help
```

O VSP só confirma a criação depois de encontrar no repositório SAP uma versão
ativa do ENHO com o subtipo correto. Se houver erro, resultado parcial ou
timeout, a operação não é apresentada como sucesso.

## Criar um XH

Para um source-code plug-in, informe o programa hospedeiro e o `FULL_NAME`
exato do ponto de enhancement. O VSP não tenta adivinhar o ponto de injeção.

```powershell
@'
DATA lv_sample TYPE string.
lv_sample = 'synthetic'.
'@ | vsp -s dev enhancement create xh ZSAMPLE_XH `
  --host-type PROG `
  --host ZSAMPLE_HOST `
  --anchor '\PR:ZSAMPLE_HOST\SE:END\EI' `
  --package '$TMP' `
  --description 'Synthetic source hook'
```

O conteúdo recebido por stdin é o corpo da implementação. Os campos
`--program`, `--main-type` e `--main-name` assumem os valores do host quando
omitidos. Os modos válidos de `--enhancement-mode` são `S`, `E` e `I`.

## Criar um class enhancement

A classe hospedeira precisa existir e estar ativa. É possível criar somente o
container ou adicionar um método de instância sem parâmetros.

```powershell
@'
DATA lv_sample TYPE string.
lv_sample = 'synthetic'.
'@ | vsp -s dev enhancement create class ZSAMPLE_CLASS_ENH `
  --class ZCL_SAMPLE_HOST `
  --method SAMPLE_METHOD `
  --method-description 'Synthetic enhanced method' `
  --exposure PUBLIC `
  --package '$TMP' `
  --description 'Synthetic class enhancement'
```

O stdin pode conter apenas o corpo ou um bloco completo
`METHOD ... ENDMETHOD`. As exposições aceitas são `PUBLIC`, `PROTECTED` e
`PRIVATE`. Sem `--method`, o VSP cria o class enhancement vazio.

Escopo atual: um método por operação, sem parâmetros, exceções, atributos,
eventos ou adição de interfaces.

## Criar uma implementação de BAdI

O enhancement spot, a definição da BAdI e a classe implementadora devem existir.
A classe também deve implementar a interface exigida pela definição. O VSP cria
a ligação no ENHO, mas não altera a classe.

```powershell
vsp -s dev enhancement create badi ZSAMPLE_BADI_ENH `
  --spot ZSAMPLE_SPOT `
  --badi ZSAMPLE_BADI `
  --implementation ZSAMPLE_IMPL `
  --implementation-class ZCL_SAMPLE_IMPL `
  --implementation-description 'Synthetic implementation' `
  --package '$TMP' `
  --description 'Synthetic BAdI implementation'
```

`--inactive` cria a entrada inativa. `--default` marca a implementação como
padrão. Filtros, geração automática da classe e criação da definição/spot ainda
não fazem parte deste fluxo.

## Usar pacote transportável

Em pacote transportável, informe sempre o transporte e habilite explicitamente
a edição. Recomenda-se restringir também os pacotes e transportes permitidos.

```powershell
vsp -s dev enhancement create class ZSAMPLE_CLASS_ENH `
  --class ZCL_SAMPLE_HOST `
  --package ZSAMPLE `
  --description 'Synthetic transported enhancement' `
  --transport DEVK900001 `
  --allow-transportable-edits `
  --allowed-packages 'ZSAMPLE' `
  --allowed-transports 'DEVK9*'
```

Também é possível configurar `SAP_ALLOW_TRANSPORTABLE_EDITS`,
`SAP_ALLOWED_PACKAGES` e `SAP_ALLOWED_TRANSPORTS`, ou os campos equivalentes do
perfil `.vsp.json`.

A versão atual corrige o erro que ocorria em sistemas clássicos quando o SAP
tentava abrir a seleção interativa de transporte dentro do worker em background.
Se aparecer `TRINT_ORDER_CHOICE` ou `DYNPRO_SEND_IN_BACKGROUND`, atualize o
binário e reinstale o bridge.

## Conferir o que foi criado

```powershell
vsp -s dev source ENHO ZSAMPLE_XH
vsp -s dev source ENHO ZSAMPLE_CLASS_ENH
vsp -s dev source ENHO ZSAMPLE_BADI_ENH
```

- XH retorna o wrapper e o código da implementação.
- Class enhancement retorna os includes gerados de declaração e métodos.
- BAdI retorna metadados estruturados da definição, implementação, classe e
  estado de ativação.

O VSP consulta o tipo real em `ENHHEADER.ENHTOOLTYPE`, evitando a classificação
incorreta como XH encontrada em alguns serviços ADT antigos.

## Erros mais comuns

| Mensagem ou sintoma | O que conferir |
|---|---|
| Bridge sem suporte à criação | Reinstale ZADT_VSP com o binário atual |
| Pacote exige transporte | Passe `--transport` e `--allow-transportable-edits` |
| Transporte não permitido | Confira a allowlist e a ordem/tarefa aprovada |
| ENHO já existe | Use outro nome; criação não sobrescreve objetos |
| Classe não encontrada | Crie e ative a classe hospedeira/implementadora primeiro |
| Anchor inválido | Use o `FULL_NAME` exato fornecido pelo SAP |
| ENHO não apareceu ativo em 60 segundos | Verifique dumps e logs de ativação antes de repetir |
| Tipo retornado diferente do solicitado | Inspecione o ENHO existente; o VSP falha de forma segura |

## O que foi validado

Em um laboratório SAP NetWeaver 7.52 NPL isolado, usando somente objetos
sintéticos, passaram os seguintes cenários:

- criação, ativação e leitura posterior de XH em `$TMP` e pacote transportável;
- class enhancement com método e leitura dos includes gerados nos dois tipos de
  pacote;
- implementação de BAdI e leitura dos metadados estruturados nos dois tipos de
  pacote;
- confirmação de propriedade CTS nas criações transportáveis;
- testes automatizados, `go vet` e GitHub Actions.

Nenhum código, credencial, endpoint, pacote, transporte, dump ou informação de
cliente foi usado como fixture ou publicado no repositório.
