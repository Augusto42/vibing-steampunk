************************************************************************
* Function group ZADT_DEBUG — RFC surface of the debugger facade.
*
* Every module here is a thin wrapper over ZCL_ADT_DEBUG: typed scalars
* go in, one JSON string comes out. None of them RAISEs an RFC exception,
* because an RFC exception discards the exporting parameters and with them
* the message; failures come back as E_RC <> 0 plus E_MESSAGE instead.
*
* The session-bound modules (LISTEN / ATTACH / STEP / STACK / DETACH) only
* work when consecutive calls reach the same ABAP session — call them on a
* pinned open-rfc-go conversation (rfc.Client.Pin), never through the pool.
*
* All modules must be flagged "Remote-Enabled Module" (FMODE = 'R').
* The group already exists in package $ZADT_DEBUG and holds ZADT_DEBUG_LOOP,
* which stays as it is: it is the debuggee to aim breakpoints at.
************************************************************************

*----------------------------------------------------------------------*
* ZADT_DEBUG_STATE
*   EXPORTING  e_json TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_state.
  e_json = zcl_adt_debug=>state( ).
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_BP_SET
*   IMPORTING  i_program      TYPE programm
*              i_line         TYPE i
*              i_request_user TYPE xubname   OPTIONAL
*              i_condition    TYPE string    OPTIONAL
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_bp_set.
  TRY.
      e_json = zcl_adt_debug=>bp_set(
        i_program      = i_program
        i_line         = i_line
        i_request_user = i_request_user
        i_condition    = i_condition ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_BP_LIST
*   IMPORTING  i_request_user TYPE xubname OPTIONAL
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_bp_list.
  TRY.
      e_json = zcl_adt_debug=>bp_list( i_request_user = i_request_user ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_BP_DEL
*   IMPORTING  i_program      TYPE programm OPTIONAL
*              i_line         TYPE i        OPTIONAL
*              i_request_user TYPE xubname  OPTIONAL
*              i_all          TYPE xfeld    OPTIONAL
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_bp_del.
  TRY.
      e_json = zcl_adt_debug=>bp_delete(
        i_program      = i_program
        i_line         = i_line
        i_request_user = i_request_user
        i_all          = COND #( WHEN i_all IS INITIAL THEN abap_false ELSE abap_true ) ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_LISTEN  — blocks up to I_TIMEOUT seconds. Raise the client
*   call timeout accordingly (rfc --timeout, vsp --rfc-timeout).
*   IMPORTING  i_request_user TYPE xubname OPTIONAL
*              i_timeout      TYPE i DEFAULT 60
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_listen.
  TRY.
      e_json = zcl_adt_debug=>listen(
        i_request_user = i_request_user
        i_timeout      = i_timeout ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_ATTACH
*   IMPORTING  i_debuggee_id TYPE sysuuid_c32
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_attach.
  TRY.
      e_json = zcl_adt_debug=>attach( i_debuggee_id = i_debuggee_id ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_STEP
*   IMPORTING  i_kind TYPE char10 DEFAULT 'into'   " into|over|out|continue
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_step.
  TRY.
      e_json = zcl_adt_debug=>step( i_kind = i_kind ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_STACK
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_stack.
  TRY.
      e_json = zcl_adt_debug=>stack( ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.

*----------------------------------------------------------------------*
* ZADT_DEBUG_DETACH
*   EXPORTING  e_json TYPE string  e_rc TYPE i  e_message TYPE string
*----------------------------------------------------------------------*
FUNCTION zadt_debug_detach.
  TRY.
      e_json = zcl_adt_debug=>detach( ).
    CATCH cx_root INTO DATA(lx_error).
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
ENDFUNCTION.
