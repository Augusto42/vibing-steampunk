* ZADT_DEBUG_STATE — the roll-area probe on its own module, so it can be
* remote-enabled independently of the facade and used as the cheapest possible
* "did my pinned session hold?" check. Two calls on one pinned RFC session
* return the same "roll" and a rising "calls"; two calls through the pool do
* not. The state deliberately hangs off a local class of its own rather than
* the facade's, because this include is compiled before the facade's.

CLASS lcl_state DEFINITION FINAL CREATE PRIVATE.
  PUBLIC SECTION.
    CLASS-DATA gv_calls TYPE i.
    CLASS-DATA gv_roll  TYPE string.
ENDCLASS.

FUNCTION ZADT_DEBUG_STATE
  EXPORTING
    VALUE(e_json) TYPE string.

  lcl_state=>gv_calls = lcl_state=>gv_calls + 1.
  IF lcl_state=>gv_roll IS INITIAL.
    TRY.
        lcl_state=>gv_roll = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lcl_state=>gv_roll = |{ sy-datum }{ sy-uzeit }|.
    ENDTRY.
  ENDIF.

  e_json = |\{"roll":"{ lcl_state=>gv_roll }","calls":{ lcl_state=>gv_calls },"uname":"{ sy-uname }"\}|.

ENDFUNCTION.
