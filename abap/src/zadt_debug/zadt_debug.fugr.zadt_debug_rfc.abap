************************************************************************
* Debugger facade for the classic-RFC leg of vibing-steampunk.
*
* Everything the ABAP debugger needs is reachable through IF_TPDAPI_*,
* but the session part of it only works when consecutive calls land in
* the SAME ABAP roll area: ATTACH_DEBUGGEE returns an object reference,
* and STEP / STACK hang off that reference. A pinned classic-RFC
* conversation gives exactly that, which is why this facade keeps its
* state in CLASS-DATA rather than handing ids back to the caller.
*
* One entry point rather than one module per operation, because the
* Remote-Enabled flag is the one thing ADT cannot set — so the fewer
* modules that need it set by hand, the better. Typed scalars go in
* (nothing here parses JSON), and one JSON string comes out (so no DDIC
* structure has to be created for every payload shape).
************************************************************************

CLASS lcl_dbg DEFINITION FINAL CREATE PRIVATE.

  PUBLIC SECTION.

    "! Roll-area probe. Two consecutive calls on one pinned RFC session
    "! return the same "roll" and an increasing "calls" — on separate
    "! calls through the pool they do not.
    CLASS-METHODS state
      RETURNING VALUE(r_json) TYPE string .

    CLASS-METHODS bp_set
      IMPORTING i_program      TYPE csequence
                i_line         TYPE i
                i_include      TYPE csequence OPTIONAL
                i_request_user TYPE csequence OPTIONAL
                i_condition    TYPE csequence OPTIONAL
      RETURNING VALUE(r_json)  TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS bp_list
      IMPORTING i_request_user TYPE csequence OPTIONAL
      RETURNING VALUE(r_json)  TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS bp_delete
      IMPORTING i_program      TYPE csequence OPTIONAL
                i_line         TYPE i         OPTIONAL
                i_request_user TYPE csequence OPTIONAL
                i_all          TYPE abap_bool DEFAULT abap_false
      RETURNING VALUE(r_json)  TYPE string
      RAISING   cx_static_check .

    "! Blocks until a debuggee of I_REQUEST_USER stops, or the timeout
    "! expires. Meant to be called on a pinned session.
    CLASS-METHODS listen
      IMPORTING i_request_user TYPE csequence OPTIONAL
                i_timeout      TYPE i DEFAULT 60
      RETURNING VALUE(r_json)  TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS attach
      IMPORTING i_debuggee_id TYPE csequence
      RETURNING VALUE(r_json) TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS step
      IMPORTING i_kind        TYPE csequence DEFAULT 'into'
      RETURNING VALUE(r_json) TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS stack
      RETURNING VALUE(r_json) TYPE string
      RAISING   cx_static_check .

    CLASS-METHODS detach
      RETURNING VALUE(r_json) TYPE string
      RAISING   cx_static_check .

  PRIVATE SECTION.

    TYPES: BEGIN OF ty_state,
             roll        TYPE string,
             calls       TYPE i,
             since       TYPE string,
             uname       TYPE syuname,
             sysid       TYPE sysysid,
             available   TYPE abap_bool,
             activated   TYPE abap_bool,
             bp_context  TYPE abap_bool,
             attached    TYPE abap_bool,
             debuggee_id TYPE string,
           END OF ty_state .

    TYPES: BEGIN OF ty_position,
             program  TYPE string,
             include  TYPE string,
             line     TYPE i,
             procname TYPE string,
           END OF ty_position .

    CLASS-DATA gv_roll       TYPE string .
    CLASS-DATA gv_calls      TYPE i .
    CLASS-DATA gv_since      TYPE string .
    CLASS-DATA go_svc        TYPE REF TO if_tpdapi_service .
    CLASS-DATA go_session    TYPE REF TO if_tpdapi_session .
    CLASS-DATA go_bp         TYPE REF TO if_tpdapi_static_bp_services .
    CLASS-DATA gv_bp_context TYPE abap_bool .
    CLASS-DATA gv_activated  TYPE abap_bool .
    CLASS-DATA gv_debuggee   TYPE string .

    CLASS-METHODS touch .
    CLASS-METHODS service
      RETURNING VALUE(r_svc) TYPE REF TO if_tpdapi_service .
    CLASS-METHODS bp_services
      IMPORTING i_request_user TYPE csequence
      RETURNING VALUE(r_bp)    TYPE REF TO if_tpdapi_static_bp_services
      RAISING   cx_static_check .
    CLASS-METHODS session
      RETURNING VALUE(r_session) TYPE REF TO if_tpdapi_session
      RAISING   cx_static_check .
    CLASS-METHODS current_position
      RETURNING VALUE(r_position) TYPE ty_position
      RAISING   cx_static_check .
    CLASS-METHODS json
      IMPORTING i_data        TYPE any
      RETURNING VALUE(r_json) TYPE string .
    CLASS-METHODS user
      IMPORTING i_user        TYPE csequence
      RETURNING VALUE(r_user) TYPE syuname .

ENDCLASS.



CLASS lcl_dbg IMPLEMENTATION.


  METHOD touch.
    gv_calls = gv_calls + 1.
    IF gv_roll IS INITIAL.
      TRY.
          gv_roll = cl_system_uuid=>create_uuid_c32_static( ).
        CATCH cx_uuid_error.
          gv_roll = |{ sy-datum }{ sy-uzeit }|.
      ENDTRY.
      gv_since = |{ sy-datum }T{ sy-uzeit }|.
    ENDIF.
  ENDMETHOD.


  METHOD json.
    r_json = /ui2/cl_json=>serialize(
      data        = i_data
      pretty_name = /ui2/cl_json=>pretty_mode-low_case ).
  ENDMETHOD.


  METHOD user.
    r_user = to_upper( i_user ).
    IF r_user IS INITIAL.
      r_user = sy-uname.
    ENDIF.
  ENDMETHOD.


  METHOD service.
    IF go_svc IS INITIAL.
      go_svc = cl_tpdapi_service=>s_get_instance( ).
    ENDIF.
    r_svc = go_svc.
  ENDMETHOD.


  METHOD session.
    IF go_session IS INITIAL.
      RAISE EXCEPTION TYPE cx_tpdapi_not_attached.
    ENDIF.
    r_session = go_session.
  ENDMETHOD.


  METHOD bp_services.
    IF go_bp IS INITIAL.
      go_bp = service( )->get_static_bp_services( ).
    ENDIF.
    IF gv_bp_context = abap_false.
      go_bp->set_external_bp_context_user(
        i_ide_user     = sy-uname
        i_request_user = user( i_request_user ) ).
      gv_bp_context = abap_true.
    ENDIF.
    r_bp = go_bp.
  ENDMETHOD.


  METHOD state.
    touch( ).
    DATA(ls_state) = VALUE ty_state(
      roll        = gv_roll
      calls       = gv_calls
      since       = gv_since
      uname       = sy-uname
      sysid       = sy-sysid
      available   = COND #( WHEN cl_tpdapi_service=>is_debugging_available( ) IS NOT INITIAL
                            THEN abap_true ELSE abap_false )
      activated   = gv_activated
      bp_context  = gv_bp_context
      attached    = COND #( WHEN go_session IS BOUND THEN abap_true ELSE abap_false )
      debuggee_id = gv_debuggee ).
    r_json = json( ls_state ).
  ENDMETHOD.


  METHOD bp_set.
    touch( ).
    DATA(lv_user) = user( i_request_user ).
    " A line number alone is read against the main program, so a breakpoint
    " inside a function module or a class method needs its include named too.
    " CREATE_LINE_BREAKPOINT submits by itself unless told otherwise; with a
    " condition we have to hold that back, because the condition must be on the
    " breakpoint before it is submitted.
    DATA(lv_submit) = COND xflag( WHEN i_condition IS INITIAL THEN 'X' ELSE space ).
    DATA(lo_bp) = CAST if_tpdapi_bp_modify(
      bp_services( lv_user )->create_line_breakpoint(
        i_main_program = to_upper( i_program )
        i_include      = COND string( WHEN i_include IS INITIAL
                                      THEN to_upper( i_program ) ELSE to_upper( i_include ) )
        i_line_nr      = i_line
        i_flg_submit   = lv_submit ) ).

    IF i_condition IS NOT INITIAL.
      lo_bp->set_condition( i_condition = i_condition ).
      lo_bp->submit( ).
    ENDIF.

    DATA: BEGIN OF ls_out,
            id        TYPE string,
            program   TYPE string,
            line      TYPE i,
            user      TYPE syuname,
            condition TYPE string,
            active    TYPE abap_bool,
          END OF ls_out.
    ls_out-id        = lo_bp->get_id( ).
    ls_out-program   = to_upper( i_program ).
    ls_out-line      = i_line.
    ls_out-user      = lv_user.
    ls_out-condition = i_condition.
    " IS_ACTIVE is not among the aliases IF_TPDAPI_BP_MODIFY lifts from
    " IF_TPDAPI_BP, so it has to be called through the interface.
    ls_out-active    = COND #( WHEN lo_bp->if_tpdapi_bp~is_active( ) IS NOT INITIAL
                               THEN abap_true ELSE abap_false ).
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD bp_list.
    touch( ).
    DATA(lv_user) = user( i_request_user ).
    DATA(lt_bps) = bp_services( lv_user )->get_breakpoints( i_flg_initialize = 'X' ).

    TYPES: BEGIN OF ty_bp,
             id         TYPE string,
             kind       TYPE string,
             serialized TYPE string,
             condition  TYPE string,
             skipcount  TYPE i,
             active     TYPE abap_bool,
           END OF ty_bp.
    DATA lt_out TYPE STANDARD TABLE OF ty_bp WITH DEFAULT KEY.

    LOOP AT lt_bps INTO DATA(lo_bp).
      DATA(ls_bp) = VALUE ty_bp( id = lo_bp->get_id( ) ).
      TRY.
          ls_bp-serialized = lo_bp->serialize( ).
        CATCH cx_tpdapi_failure ##NO_HANDLER.
      ENDTRY.
      TRY.
          ls_bp-condition = lo_bp->get_condition( ).
        CATCH cx_tpdapi_not_supported cx_tpdapi_invalidated ##NO_HANDLER.
      ENDTRY.
      TRY.
          ls_bp-skipcount = lo_bp->get_skipcount( ).
          ls_bp-active    = COND #( WHEN lo_bp->is_active( ) IS NOT INITIAL THEN abap_true ELSE abap_false ).
        CATCH cx_tpdapi_invalidated ##NO_HANDLER.
      ENDTRY.
      APPEND ls_bp TO lt_out.
    ENDLOOP.
    r_json = json( lt_out ).
  ENDMETHOD.


  METHOD bp_delete.
    touch( ).
    DATA(lv_user) = user( i_request_user ).
    DATA(lo_services) = bp_services( lv_user ).
    DATA lv_deleted TYPE i.

    LOOP AT lo_services->get_breakpoints( i_flg_initialize = 'X' ) INTO DATA(lo_bp).
      DATA(lv_hit) = i_all.
      IF lv_hit = abap_false AND i_program IS NOT INITIAL.
        TRY.
            DATA(lv_text) = lo_bp->serialize( ).
            IF lv_text CS to_upper( i_program )
               AND ( i_line IS INITIAL OR lv_text CS |{ i_line }| ).
              lv_hit = abap_true.
            ENDIF.
          CATCH cx_tpdapi_failure ##NO_HANDLER.
        ENDTRY.
      ENDIF.
      IF lv_hit = abap_true.
        CAST if_tpdapi_bp_modify( lo_bp )->delete( ).
        lv_deleted = lv_deleted + 1.
      ENDIF.
    ENDLOOP.

    DATA: BEGIN OF ls_out,
            deleted TYPE i,
            user    TYPE syuname,
          END OF ls_out.
    ls_out-deleted = lv_deleted.
    ls_out-user    = lv_user.
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD listen.
    touch( ).
    DATA(lv_user) = user( i_request_user ).
    DATA(lv_timeout) = i_timeout.
    IF lv_timeout <= 0.
      lv_timeout = 60.
    ELSEIF lv_timeout > 240.
      lv_timeout = 240.
    ENDIF.

    DATA(lo_svc) = service( ).
    lo_svc->activate_session_for_ext_debug( i_ide_user = sy-uname ).
    gv_activated = abap_true.

    DATA lv_status TYPE string VALUE 'stopped'.
    TRY.
        lo_svc->start_listener_for_user(
          i_request_user = lv_user
          i_ide_user     = sy-uname
          i_timeout      = lv_timeout ).
      CATCH cx_abdbg_actext_lis_timeout.
        lv_status = 'timeout'.
    ENDTRY.

    DATA(lt_debuggees) = lo_svc->get_waiting_debuggees(
      i_request_user = lv_user
      i_ide_user     = sy-uname ).

    DATA: BEGIN OF ls_out,
            status    TYPE string,
            user      TYPE syuname,
            timeout   TYPE i,
            debuggees TYPE if_tpdapi_service=>typ_tab_debuggees,
          END OF ls_out.
    ls_out-status    = lv_status.
    ls_out-user      = lv_user.
    ls_out-timeout   = lv_timeout.
    ls_out-debuggees = lt_debuggees.
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD attach.
    touch( ).
    IF go_session IS BOUND.
      RAISE EXCEPTION TYPE cx_tpdapi_failure.
    ENDIF.
    " A session may only attach once external debugging is activated for it.
    " LISTEN does that on the way in, but a bare ATTACH — reconnecting to a
    " debuggee someone else caught, or one found by polling ABDBG_ACTIVATION —
    " would otherwise fail with a bare CX_TPDAPI_FAILURE.
    IF gv_activated = abap_false.
      service( )->activate_session_for_ext_debug( i_ide_user = sy-uname ).
      gv_activated = abap_true.
    ENDIF.
    go_session = service( )->attach_debuggee( i_debuggee_id = CONV #( i_debuggee_id ) ).
    gv_debuggee = i_debuggee_id.

    DATA: BEGIN OF ls_out,
            attached     TYPE abap_bool,
            debuggee_id  TYPE string,
            session_id   TYPE string,
            is_rfc       TYPE abap_bool,
            post_mortem  TYPE abap_bool,
            position     TYPE ty_position,
          END OF ls_out.
    ls_out-attached    = abap_true.
    ls_out-debuggee_id = i_debuggee_id.
    ls_out-session_id  = go_session->get_session_id( ).
    ls_out-is_rfc      = COND #( WHEN go_session->is_rfc( ) IS NOT INITIAL THEN abap_true ELSE abap_false ).
    ls_out-post_mortem = COND #( WHEN go_session->is_post_mortem( ) IS NOT INITIAL THEN abap_true ELSE abap_false ).
    ls_out-position    = current_position( ).
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD step.
    touch( ).
    DATA lo_steptype TYPE REF TO ie_tpdapi_steptype.
    CASE to_lower( i_kind ).
      WHEN 'into'.     lo_steptype = ce_tpdapi_steptype=>into.
      WHEN 'over'.     lo_steptype = ce_tpdapi_steptype=>over.
      WHEN 'out' OR 'return'. lo_steptype = ce_tpdapi_steptype=>out.
      WHEN 'continue'. lo_steptype = ce_tpdapi_steptype=>continue.
      WHEN OTHERS.
        RAISE EXCEPTION TYPE cx_tpdapi_invalid_param.
    ENDCASE.

    session( )->get_control_services( )->debug_step( i_ref_steptype = lo_steptype ).

    DATA: BEGIN OF ls_out,
            kind     TYPE string,
            position TYPE ty_position,
          END OF ls_out.
    ls_out-kind     = to_lower( i_kind ).
    ls_out-position = current_position( ).
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD current_position.
    LOOP AT session( )->get_stack_handler( )->get_stack( ) INTO DATA(ls_frame).
      IF ls_frame-flg_active IS NOT INITIAL.
        r_position-program  = ls_frame-program.
        r_position-include  = ls_frame-include.
        r_position-line     = ls_frame-line.
        r_position-procname = ls_frame-procname.
        RETURN.
      ENDIF.
    ENDLOOP.
  ENDMETHOD.


  METHOD stack.
    touch( ).
    " Project the stack onto the five fields anyone actually reads. Handing the
    " raw TPDAPI stack table to /UI2/CL_JSON hangs the call: the line type
    " carries more than plain scalars, and the caller then waits out its whole
    " RFC timeout with the debuggee still attached.
    TYPES: BEGIN OF ty_frame,
             level    TYPE i,
             program  TYPE string,
             include  TYPE string,
             line     TYPE i,
             procname TYPE string,
             active   TYPE abap_bool,
           END OF ty_frame.
    DATA lt_frames TYPE STANDARD TABLE OF ty_frame WITH DEFAULT KEY.
    DATA lv_level TYPE i.

    LOOP AT session( )->get_stack_handler( )->get_stack( ) INTO DATA(ls_frame).
      lv_level = lv_level + 1.
      APPEND VALUE ty_frame(
        level    = lv_level
        program  = ls_frame-program
        include  = ls_frame-include
        line     = ls_frame-line
        procname = ls_frame-procname
        active   = COND #( WHEN ls_frame-flg_active IS NOT INITIAL
                           THEN abap_true ELSE abap_false ) ) TO lt_frames.
    ENDLOOP.

    DATA: BEGIN OF ls_out,
            debuggee_id TYPE string,
            frames      TYPE STANDARD TABLE OF ty_frame WITH DEFAULT KEY,
          END OF ls_out.
    ls_out-debuggee_id = gv_debuggee.
    ls_out-frames      = lt_frames.
    r_json = json( ls_out ).
  ENDMETHOD.


  METHOD detach.
    touch( ).
    DATA lv_ended TYPE abap_bool.
    IF go_session IS BOUND.
      TRY.
          go_session->get_control_services( )->end_debugger( ).
          lv_ended = abap_true.
        CATCH cx_tpdapi_failure ##NO_HANDLER.   " NOT_ATTACHED is a subclass
      ENDTRY.
      CLEAR go_session.
    ENDIF.
    " Always try to stop the listener, not only when this session started one.
    " A conversation that died mid-call (END_DEBUGGER closes it, and so does any
    " transport failure) leaves its ABDBG_LISTENER row behind, and that row then
    " conflicts with the next listener. DETACH is the broom.
    TRY.
        service( )->stop_listener_for_user( i_request_user = sy-uname ).
      CATCH cx_tpdapi_failure ##NO_HANDLER.
    ENDTRY.
    gv_activated = abap_false.
    CLEAR: gv_debuggee, gv_bp_context, go_bp.

    DATA: BEGIN OF ls_out,
            detached TYPE abap_bool,
            ended    TYPE abap_bool,
          END OF ls_out.
    ls_out-detached = abap_true.
    ls_out-ended    = lv_ended.
    r_json = json( ls_out ).
  ENDMETHOD.


ENDCLASS.


FUNCTION ZADT_DEBUG_RFC
  IMPORTING
    VALUE(i_op) TYPE char20
    VALUE(i_program) TYPE programm OPTIONAL
    VALUE(i_include) TYPE programm OPTIONAL
    VALUE(i_line) TYPE i OPTIONAL
    VALUE(i_user) TYPE xubname OPTIONAL
    VALUE(i_debuggee_id) TYPE sysuuid_c32 OPTIONAL
    VALUE(i_kind) TYPE char10 OPTIONAL
    VALUE(i_timeout) TYPE i DEFAULT 60
    VALUE(i_condition) TYPE string OPTIONAL
    VALUE(i_all) TYPE xfeld OPTIONAL
  EXPORTING
    VALUE(e_json) TYPE string
    VALUE(e_rc) TYPE i
    VALUE(e_message) TYPE string.

  TRY.
      CASE to_lower( i_op ).
        WHEN 'state'.
          e_json = lcl_dbg=>state( ).
        WHEN 'bp_set'.
          e_json = lcl_dbg=>bp_set( i_program      = i_program
                                    i_line         = i_line
                                    i_include      = i_include
                                    i_request_user = i_user
                                    i_condition    = i_condition ).
        WHEN 'bp_list'.
          e_json = lcl_dbg=>bp_list( i_request_user = i_user ).
        WHEN 'bp_delete'.
          e_json = lcl_dbg=>bp_delete( i_program      = i_program
                                       i_line         = i_line
                                       i_request_user = i_user
                                       i_all          = COND #( WHEN i_all IS INITIAL
                                                                THEN abap_false ELSE abap_true ) ).
        WHEN 'listen'.
          e_json = lcl_dbg=>listen( i_request_user = i_user
                                    i_timeout      = i_timeout ).
        WHEN 'attach'.
          e_json = lcl_dbg=>attach( i_debuggee_id = i_debuggee_id ).
        WHEN 'step'.
          e_json = lcl_dbg=>step( i_kind = COND #( WHEN i_kind IS INITIAL THEN 'into' ELSE i_kind ) ).
        WHEN 'stack'.
          e_json = lcl_dbg=>stack( ).
        WHEN 'detach'.
          e_json = lcl_dbg=>detach( ).
        WHEN OTHERS.
          e_rc      = 8.
          e_message = |unknown op '{ i_op }'; use state, bp_set, bp_list, bp_delete, | &&
                      |listen, attach, step, stack, detach|.
      ENDCASE.

    CATCH cx_root INTO DATA(lx_error).
      " An RFC exception would discard the exporting parameters and with them
      " the message, so failures are reported in E_RC / E_MESSAGE instead.
      e_rc      = 4.
      e_message = lx_error->get_text( ).
  ENDTRY.
