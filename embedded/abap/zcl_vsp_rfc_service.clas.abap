"! <p class="shorttext synchronized">VSP RFC Service</p>
"! Enables dynamic RFC/BAPI calls via WebSocket.
"! Actions: call, search, getMetadata, ping, moveToPackage, runReport, readSource,
"!          writeEnhancementSource
CLASS zcl_vsp_rfc_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.
    CLASS-METHODS class_constructor.
    CLASS-METHODS write_enho_worker
      IMPORTING iv_enhname   TYPE string
                iv_source_b64 TYPE string
                iv_transport TYPE string.

  PRIVATE SECTION.
    TYPES:
      BEGIN OF ty_param_info,
        parameter  TYPE rs38l_par_,
        kind       TYPE rs38l_kind,
        typ        TYPE string,
        dbfield    TYPE string,
        optional   TYPE rs38l_opti,
      END OF ty_param_info,
      tt_param_info TYPE STANDARD TABLE OF ty_param_info WITH KEY parameter.

    METHODS handle_call
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_search
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_metadata
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_ping
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_move_to_package
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_run_report
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Read source of any program/include using native READ REPORT.
    "! Works for SUBC=I includes (e.g. enhancement plug-in source) where
    "! RPY_PROGRAM_READ raises CANCELLED in RFC contexts.
    METHODS handle_read_source
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Update and activate an existing classic HOOK_IMPL enhancement.
    METHODS handle_write_enho_source
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS get_func_interface
      IMPORTING iv_function       TYPE rs38l_fnam
      EXPORTING et_import         TYPE tt_param_info
                et_export         TYPE tt_param_info
                et_changing       TYPE tt_param_info
                et_tables         TYPE tt_param_info
      RETURNING VALUE(rv_success) TYPE abap_bool.

    METHODS create_param_data
      IMPORTING is_param       TYPE ty_param_info
      RETURNING VALUE(ro_data) TYPE REF TO data.

    METHODS create_table_data
      IMPORTING is_param       TYPE ty_param_info
      RETURNING VALUE(ro_data) TYPE REF TO data.

    METHODS extract_param
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE string.

    METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    METHODS build_error
      IMPORTING iv_id            TYPE string
                iv_code          TYPE string
                iv_message       TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    CLASS-DATA gv_pcre_supported TYPE abap_bool.
    CLASS-DATA gv_enho_worker_mode TYPE abap_bool.

ENDCLASS.


CLASS zcl_vsp_rfc_service IMPLEMENTATION.

  METHOD class_constructor.
    " Probe for PCRE support: \d was introduced in ABAP 7.55 (POSIX ERE on older releases).
    TRY.
        DATA lv_pcre_probe TYPE string.
        FIND REGEX '\d' IN '1' SUBMATCHES lv_pcre_probe.
        gv_pcre_supported = xsdbool( sy-subrc = 0 AND lv_pcre_probe = '1' ).
      CATCH cx_root.
        gv_pcre_supported = abap_false.
    ENDTRY.
  ENDMETHOD.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'rfc'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'call'.
        rs_response = handle_call( is_message ).
      WHEN 'search'.
        rs_response = handle_search( is_message ).
      WHEN 'getMetadata'.
        rs_response = handle_get_metadata( is_message ).
      WHEN 'ping'.
        rs_response = handle_ping( is_message ).
      WHEN 'moveToPackage'.
        rs_response = handle_move_to_package( is_message ).
      WHEN 'runReport'.
        rs_response = handle_run_report( is_message ).
      WHEN 'readSource'.
        rs_response = handle_read_source( is_message ).
      WHEN 'writeEnhancementSource'.
        rs_response = handle_write_enho_source( is_message ).
      WHEN OTHERS.
        rs_response = build_error(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = 'Action not supported'
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
  ENDMETHOD.

  METHOD handle_move_to_package.
    " Extract parameters: pgmid, object, obj_name, new_package
    DATA(lv_pgmid) = extract_param( iv_params = is_message-params iv_name = 'pgmid' ).
    DATA(lv_object) = extract_param( iv_params = is_message-params iv_name = 'object' ).
    DATA(lv_obj_name) = extract_param( iv_params = is_message-params iv_name = 'obj_name' ).
    DATA(lv_new_pkg) = extract_param( iv_params = is_message-params iv_name = 'new_package' ).

    " Validate required params
    IF lv_object IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter object is required (e.g., CLAS, PROG, INTF, SAPC)' ).
      RETURN.
    ENDIF.
    IF lv_obj_name IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter obj_name is required' ).
      RETURN.
    ENDIF.
    IF lv_new_pkg IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter new_package is required' ).
      RETURN.
    ENDIF.

    " Default pgmid to R3TR
    IF lv_pgmid IS INITIAL.
      lv_pgmid = 'R3TR'.
    ENDIF.

    " Uppercase all values
    TRANSLATE lv_pgmid TO UPPER CASE.
    TRANSLATE lv_object TO UPPER CASE.
    TRANSLATE lv_obj_name TO UPPER CASE.
    TRANSLATE lv_new_pkg TO UPPER CASE.

    " Call ZADT_CL_TADIR_MOVE to perform the move
    DATA lv_result TYPE string.
    TRY.
        lv_result = zadt_cl_tadir_move=>move_object_and_commit(
          iv_pgmid    = CONV #( lv_pgmid )
          iv_object   = CONV #( lv_object )
          iv_obj_name = CONV #( lv_obj_name )
          iv_new_pkg  = CONV #( lv_new_pkg )
        ).
      CATCH cx_root INTO DATA(lx_error).
        rs_response = build_error( iv_id = is_message-id iv_code = 'MOVE_ERROR' iv_message = lx_error->get_text( ) ).
        RETURN.
    ENDTRY.

    " Build response
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA(lv_success) = COND string( WHEN lv_result CP 'SUCCESS*' THEN 'true' ELSE 'false' ).
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"success":{ lv_success },"pgmid":"{ lv_pgmid }","object":"{ lv_object }","obj_name":"{ lv_obj_name }","new_package":"{ lv_new_pkg }","message":"{ escape_json( lv_result ) }"{ lv_c }|.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_call.
    DATA(lv_function) = extract_param( iv_params = is_message-params iv_name = 'function' ).
    IF lv_function IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter function is required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_function TO UPPER CASE.

    DATA lt_import TYPE tt_param_info.
    DATA lt_export TYPE tt_param_info.
    DATA lt_changing TYPE tt_param_info.
    DATA lt_tables TYPE tt_param_info.

    IF get_func_interface( EXPORTING iv_function = CONV #( lv_function )
                           IMPORTING et_import = lt_import et_export = lt_export
                                     et_changing = lt_changing et_tables = lt_tables ) = abap_false.
      rs_response = build_error( iv_id = is_message-id iv_code = 'FUNC_NOT_FOUND' iv_message = |Function { lv_function } not found| ).
      RETURN.
    ENDIF.

    DATA lt_ptab TYPE abap_func_parmbind_tab.
    DATA lt_etab TYPE abap_func_excpbind_tab.
    DATA ls_ptab TYPE abap_func_parmbind.
    DATA lo_data TYPE REF TO data.
    DATA lv_val TYPE string.
    FIELD-SYMBOLS <fs_val> TYPE any.
*   classic-ECC compat: inline FIELD-SYMBOL() decls type these as `any`,
*   so LOOP AT <fs_tab> fails the syntax check. Pre-declare with explicit
*   table/structure types so the compiler is happy.
    FIELD-SYMBOLS <fs_tab> TYPE STANDARD TABLE.
    FIELD-SYMBOLS <fs_row> TYPE any.
    FIELD-SYMBOLS <fs_comp> TYPE any.
    FIELD-SYMBOLS <fs_out> TYPE any.
    FIELD-SYMBOLS <fs_exp_comp> TYPE any.

    " Function's IMPORT params: we EXPORT values TO the function
    LOOP AT lt_import INTO DATA(ls_imp).
      CLEAR: ls_ptab, lo_data, lv_val.
      DATA(lv_param_marker) = |"{ ls_imp-parameter }":|.
      FIND lv_param_marker IN is_message-params IGNORING CASE.
      IF sy-subrc <> 0.
        CONTINUE.
      ENDIF.
      lv_val = extract_param( iv_params = is_message-params iv_name = CONV #( ls_imp-parameter ) ).
      " Only bind parameters that were present in the request. Binding an
      " omitted parameter's type-initial value would override the function
      " module's declared DEFAULT value (for example JOB_CLOSE restrictions).
      lo_data = create_param_data( ls_imp ).
      IF lo_data IS BOUND.
        IF lv_val IS NOT INITIAL.
          ASSIGN lo_data->* TO <fs_val>.
          IF sy-subrc = 0.
            TRY.
                <fs_val> = lv_val.
              CATCH cx_root.
            ENDTRY.
          ENDIF.
        ENDIF.
        ls_ptab-name = ls_imp-parameter.
        ls_ptab-kind = abap_func_exporting.
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    " Function's EXPORT params: we IMPORT values FROM the function
    LOOP AT lt_export INTO DATA(ls_exp).
      CLEAR: ls_ptab, lo_data.
      lo_data = create_param_data( ls_exp ).
      IF lo_data IS BOUND.
        ls_ptab-name = ls_exp-parameter.
        ls_ptab-kind = abap_func_importing.
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    " TABLES params: create internal tables
    LOOP AT lt_tables INTO DATA(ls_tbl).
      CLEAR: ls_ptab, lo_data.
      lo_data = create_table_data( ls_tbl ).
      IF lo_data IS BOUND.
        ls_ptab-name = ls_tbl-parameter.
        ls_ptab-kind = abap_func_tables.
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    INSERT VALUE #( name = 'OTHERS' value = 99 ) INTO TABLE lt_etab.

    TRY.
        CALL FUNCTION lv_function
          PARAMETER-TABLE lt_ptab
          EXCEPTION-TABLE lt_etab.
      CATCH cx_root INTO DATA(lx_call).
        rs_response = build_error( iv_id = is_message-id iv_code = 'CALL_ERROR' iv_message = lx_call->get_text( ) ).
        RETURN.
    ENDTRY.

    DATA(lv_subrc) = sy-subrc.
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"subrc":{ lv_subrc },"exports":{ lv_o }|.

    DATA lv_first TYPE abap_bool VALUE abap_true.
    DATA lv_str TYPE string.
    LOOP AT lt_ptab INTO DATA(ls_out) WHERE kind = abap_func_importing.
      ASSIGN ls_out-value->* TO <fs_out>.
      IF sy-subrc = 0.
        IF lv_first = abap_false.
          lv_json = |{ lv_json },|.
        ENDIF.
        DATA(lv_pname) = CONV string( ls_out-name ).
        CONDENSE lv_pname.
        DATA(lo_exp_type) = cl_abap_typedescr=>describe_by_data( <fs_out> ).
        IF lo_exp_type->kind = cl_abap_typedescr=>kind_elem.
          TRY.
              lv_str = <fs_out>.
              lv_str = escape_json( lv_str ).
            CATCH cx_root.
              lv_str = ''.
          ENDTRY.
          lv_json = |{ lv_json }"{ lv_pname }":"{ lv_str }"|.
        ELSE.
          lv_json = |{ lv_json }"{ lv_pname }":{ lv_o }|.
          DATA(lo_exp_struc) = CAST cl_abap_structdescr( lo_exp_type ).
          DATA lv_exp_first TYPE abap_bool.
          lv_exp_first = abap_true.
          LOOP AT lo_exp_struc->components INTO DATA(ls_exp_comp).
            IF lv_exp_first = abap_false.
              lv_json = |{ lv_json },|.
            ENDIF.
            ASSIGN COMPONENT ls_exp_comp-name OF STRUCTURE <fs_out> TO <fs_exp_comp>.
            IF sy-subrc = 0.
              DATA(lo_comp_type) = cl_abap_typedescr=>describe_by_data( <fs_exp_comp> ).
              IF lo_comp_type->kind = cl_abap_typedescr=>kind_elem.
                TRY.
                    lv_str = <fs_exp_comp>.
                    lv_str = escape_json( lv_str ).
                  CATCH cx_root.
                    lv_str = ''.
                ENDTRY.
              ELSE.
                lv_str = '[complex]'.
              ENDIF.
              lv_json = |{ lv_json }"{ ls_exp_comp-name }":"{ lv_str }"|.
            ENDIF.
            lv_exp_first = abap_false.
          ENDLOOP.
          lv_json = |{ lv_json }{ lv_c }|.
        ENDIF.
        lv_first = abap_false.
      ENDIF.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c },"tables":{ lv_o }|.

    lv_first = abap_true.
    LOOP AT lt_ptab INTO ls_out WHERE kind = abap_func_tables.
      ASSIGN ls_out-value->* TO <fs_tab>.
      IF sy-subrc = 0.
        IF lv_first = abap_false.
          lv_json = |{ lv_json },|.
        ENDIF.
        lv_pname = CONV string( ls_out-name ).
        CONDENSE lv_pname.
        lv_json = |{ lv_json }"{ lv_pname }":[|.
        DATA lv_row_first TYPE abap_bool.
        lv_row_first = abap_true.
        TRY.
            LOOP AT <fs_tab> ASSIGNING <fs_row>.
              IF lv_row_first = abap_false.
                lv_json = |{ lv_json },|.
              ENDIF.
              lv_json = |{ lv_json }{ lv_o }|.
              DATA(lo_struc) = CAST cl_abap_structdescr( cl_abap_typedescr=>describe_by_data( <fs_row> ) ).
              DATA lv_comp_first TYPE abap_bool.
              lv_comp_first = abap_true.
              LOOP AT lo_struc->components INTO DATA(ls_comp).
                IF lv_comp_first = abap_false.
                  lv_json = |{ lv_json },|.
                ENDIF.
                ASSIGN COMPONENT ls_comp-name OF STRUCTURE <fs_row> TO <fs_comp>.
                IF sy-subrc = 0.
                  DATA(lo_type) = cl_abap_typedescr=>describe_by_data( <fs_comp> ).
                  IF lo_type->kind = cl_abap_typedescr=>kind_elem.
                    TRY.
                        lv_str = <fs_comp>.
                        lv_str = escape_json( lv_str ).
                      CATCH cx_root.
                        lv_str = ''.
                    ENDTRY.
                  ELSE.
                    lv_str = '[complex]'.
                  ENDIF.
                  lv_json = |{ lv_json }"{ ls_comp-name }":"{ lv_str }"|.
                ENDIF.
                lv_comp_first = abap_false.
              ENDLOOP.
              lv_json = |{ lv_json }{ lv_c }|.
              lv_row_first = abap_false.
            ENDLOOP.
          CATCH cx_root.
            lv_json = |{ lv_json }{ lv_o }"error":"serialization failed"{ lv_c }|.
        ENDTRY.
        lv_json = |{ lv_json }]|.
        lv_first = abap_false.
      ENDIF.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c }{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD create_param_data.
    DATA lv_type TYPE string.

    IF strlen( is_param-typ ) > 0.
      lv_type = is_param-typ.
    ELSEIF strlen( is_param-dbfield ) > 0.
      lv_type = is_param-dbfield.
    ELSE.
      CREATE DATA ro_data TYPE string.
      RETURN.
    ENDIF.

    TRY.
        CREATE DATA ro_data TYPE (lv_type).
      CATCH cx_sy_create_data_error.
        TRY.
            CREATE DATA ro_data TYPE string.
          CATCH cx_root.
            CLEAR ro_data.
        ENDTRY.
    ENDTRY.
  ENDMETHOD.

  METHOD create_table_data.
    DATA lv_type TYPE string.
    FIELD-SYMBOLS <fs_dynamic_table> TYPE ANY TABLE.

    " FUNCTION_IMPORT_INTERFACE can return either a complete DDIC table type
    " in TYP or a row structure in DBSTRUCT. Wrapping an existing table type
    " in another STANDARD TABLE creates a nested table that is incompatible
    " with PARAMETER-TABLE (for example RPY_DYNPRO_READ-CONTAINERS).
    IF strlen( is_param-typ ) > 0.
      lv_type = is_param-typ.
      TRY.
          CREATE DATA ro_data TYPE (lv_type).
          ASSIGN ro_data->* TO <fs_dynamic_table>.
          IF sy-subrc = 0.
            RETURN.
          ENDIF.
          CLEAR ro_data.
        CATCH cx_sy_create_data_error.
          CLEAR ro_data.
      ENDTRY.
    ENDIF.

    IF strlen( is_param-dbfield ) > 0.
      lv_type = is_param-dbfield.
      TRY.
          CREATE DATA ro_data TYPE STANDARD TABLE OF (lv_type).
          RETURN.
        CATCH cx_sy_create_data_error.
          CLEAR ro_data.
      ENDTRY.
    ENDIF.

    " Older function interfaces may expose only the row type in TYP.
    IF strlen( is_param-typ ) > 0.
      lv_type = is_param-typ.
      TRY.
          CREATE DATA ro_data TYPE STANDARD TABLE OF (lv_type).
        CATCH cx_sy_create_data_error.
          CLEAR ro_data.
      ENDTRY.
    ENDIF.
  ENDMETHOD.

  METHOD handle_get_metadata.
    DATA(lv_function) = extract_param( iv_params = is_message-params iv_name = 'function' ).
    IF lv_function IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter function is required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_function TO UPPER CASE.

    DATA lt_import TYPE tt_param_info.
    DATA lt_export TYPE tt_param_info.
    DATA lt_changing TYPE tt_param_info.
    DATA lt_tables TYPE tt_param_info.

    IF get_func_interface( EXPORTING iv_function = CONV #( lv_function )
                           IMPORTING et_import = lt_import et_export = lt_export
                                     et_changing = lt_changing et_tables = lt_tables ) = abap_false.
      rs_response = build_error( iv_id = is_message-id iv_code = 'FUNC_NOT_FOUND' iv_message = |Function { lv_function } not found| ).
      RETURN.
    ENDIF.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"function":"{ lv_function }","parameters":[|.

    DATA lv_first TYPE abap_bool VALUE abap_true.

    LOOP AT lt_import INTO DATA(ls_i).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      DATA(lv_typ) = COND string( WHEN strlen( ls_i-typ ) > 0 THEN ls_i-typ ELSE ls_i-dbfield ).
      DATA(lv_opt) = COND string( WHEN ls_i-optional = abap_true THEN 'true' ELSE 'false' ).
      DATA(lv_pname) = CONV string( ls_i-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"importing","type":"{ lv_typ }","optional":{ lv_opt }{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_export INTO DATA(ls_e).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_e-typ ) > 0 THEN ls_e-typ ELSE ls_e-dbfield ).
      lv_pname = CONV string( ls_e-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"exporting","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_changing INTO DATA(ls_ch).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_ch-typ ) > 0 THEN ls_ch-typ ELSE ls_ch-dbfield ).
      lv_pname = CONV string( ls_ch-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"changing","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_tables INTO DATA(ls_t).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_t-typ ) > 0 THEN ls_t-typ ELSE ls_t-dbfield ).
      lv_pname = CONV string( ls_t-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"tables","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }]{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD get_func_interface.
    rv_success = abap_false.
    CLEAR: et_import, et_export, et_changing, et_tables.

    DATA lt_import TYPE STANDARD TABLE OF rsimp.
    DATA lt_export TYPE STANDARD TABLE OF rsexp.
    DATA lt_changing TYPE STANDARD TABLE OF rscha.
    DATA lt_tables TYPE STANDARD TABLE OF rstbl.
    DATA lt_except TYPE STANDARD TABLE OF rsexc.

    CALL FUNCTION 'FUNCTION_IMPORT_INTERFACE'
      EXPORTING
        funcname           = iv_function
      TABLES
        import_parameter   = lt_import
        export_parameter   = lt_export
        changing_parameter = lt_changing
        tables_parameter   = lt_tables
        exception_list     = lt_except
      EXCEPTIONS
        error_message      = 1
        function_not_found = 2
        invalid_name       = 3
        OTHERS             = 4.

    IF sy-subrc <> 0.
      RETURN.
    ENDIF.

    LOOP AT lt_import INTO DATA(ls_i).
      INSERT VALUE #( parameter = ls_i-parameter kind = 'I' typ = ls_i-typ dbfield = ls_i-dbfield optional = ls_i-optional ) INTO TABLE et_import.
    ENDLOOP.
    LOOP AT lt_export INTO DATA(ls_e).
      INSERT VALUE #( parameter = ls_e-parameter kind = 'E' typ = ls_e-typ dbfield = ls_e-dbfield ) INTO TABLE et_export.
    ENDLOOP.
    LOOP AT lt_changing INTO DATA(ls_ch).
      INSERT VALUE #( parameter = ls_ch-parameter kind = 'C' typ = ls_ch-typ dbfield = ls_ch-dbfield optional = ls_ch-optional ) INTO TABLE et_changing.
    ENDLOOP.
    LOOP AT lt_tables INTO DATA(ls_t).
      INSERT VALUE #( parameter = ls_t-parameter kind = 'T' typ = ls_t-typ dbfield = ls_t-dbstruct optional = ls_t-optional ) INTO TABLE et_tables.
    ENDLOOP.

    rv_success = abap_true.
  ENDMETHOD.

  METHOD handle_search.
    DATA lv_pattern TYPE string.
    lv_pattern = extract_param( iv_params = is_message-params iv_name = 'pattern' ).
    IF lv_pattern IS INITIAL.
      lv_pattern = '%'.
    ENDIF.
    TRANSLATE lv_pattern TO UPPER CASE.
    REPLACE ALL OCCURRENCES OF '*' IN lv_pattern WITH '%'.

    DATA lt_funcs TYPE STANDARD TABLE OF rs38l_fnam.
    SELECT funcname FROM tfdir INTO TABLE lt_funcs
      UP TO 100 ROWS
      WHERE funcname LIKE lv_pattern
      ORDER BY funcname.

    DATA lv_json TYPE string.
    DATA lv_first TYPE abap_bool VALUE abap_true.
    lv_json = '['.
    LOOP AT lt_funcs INTO DATA(lv_func).
      IF lv_first = abap_false.
        CONCATENATE lv_json ',' INTO lv_json.
      ENDIF.
      CONCATENATE lv_json '{"name":"' lv_func '"}' INTO lv_json.
      lv_first = abap_false.
    ENDLOOP.
    CONCATENATE lv_json ']' INTO lv_json.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_ping.
    DATA lv_ts TYPE string.
    lv_ts = sy-datum && sy-uzeit.
    rs_response = VALUE #( id = is_message-id success = abap_true data = '{"pong":true,"timestamp":"' && lv_ts && '"}' ).
  ENDMETHOD.

  METHOD extract_param.
    DATA lv_name TYPE string.
    lv_name = iv_name.
    CONDENSE lv_name.

    DATA lv_regex TYPE string.
    CONCATENATE '"' lv_name '":' INTO lv_regex.
    DATA lv_pos TYPE i.
    FIND lv_regex IN iv_params MATCH OFFSET lv_pos.
    IF sy-subrc = 0.
      DATA lv_rest TYPE string.
      lv_rest = iv_params+lv_pos.
      IF gv_pcre_supported = abap_true.
        FIND REGEX ':\s*"([^"]*)"' IN lv_rest SUBMATCHES rv_value.
      ELSE.
        FIND REGEX ':[[:space:]]*"([^"]*)"' IN lv_rest SUBMATCHES rv_value.
      ENDIF.
    ENDIF.
  ENDMETHOD.

  METHOD escape_json.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '\' IN rv_escaped WITH '\\'.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>horizontal_tab IN rv_escaped WITH '\t'.
  ENDMETHOD.

  METHOD build_error.
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    rs_response = VALUE #(
      id      = iv_id
      success = abap_false
      error   = |{ lv_o }"code":"{ iv_code }","message":"{ escape_json( iv_message ) }"{ lv_c }|
    ).
  ENDMETHOD.

  METHOD handle_run_report.
    " Run a report via async RFC - workaround for APC_ILLEGAL_STATEMENT with SUBMIT
    " Uses STARTING NEW TASK to run in separate work process
    DATA: lv_report  TYPE progname,
          lv_variant TYPE raldb_vari,
          lv_task    TYPE string.

    DATA(lv_report_str) = extract_param( iv_params = is_message-params iv_name = 'report' ).
    DATA(lv_variant_str) = extract_param( iv_params = is_message-params iv_name = 'variant' ).

    IF lv_report_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter report is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_report_str TO UPPER CASE.
    lv_report = lv_report_str.

    IF lv_variant_str IS NOT INITIAL.
      TRANSLATE lv_variant_str TO UPPER CASE.
      lv_variant = lv_variant_str.
    ENDIF.

    " Verify report exists
    SELECT SINGLE name FROM trdir INTO @DATA(lv_exists) WHERE name = @lv_report.
    IF sy-subrc <> 0.
      rs_response = build_error( iv_id = is_message-id iv_code = 'REPORT_NOT_FOUND' iv_message = |Report { lv_report } not found| ).
      RETURN.
    ENDIF.

    " Generate unique task name
    lv_task = |VSP_{ sy-uzeit }|.

    " Call RFC async to run in separate work process
    " This avoids APC_ILLEGAL_STATEMENT since SUBMIT happens in different process
    TRY.
        CALL FUNCTION 'RFC_ABAP_INSTALL_AND_RUN'
          STARTING NEW TASK lv_task
          EXPORTING
            program_name = lv_report
          EXCEPTIONS
            system_failure        = 1
            communication_failure = 2
            resource_failure      = 3
            OTHERS                = 4.

        IF sy-subrc <> 0.
          rs_response = build_error( iv_id = is_message-id iv_code = 'RFC_ERROR' iv_message = |Async RFC failed: { sy-subrc }| ).
          RETURN.
        ENDIF.

      CATCH cx_root INTO DATA(lx_error).
        rs_response = build_error( iv_id = is_message-id iv_code = 'RFC_ERROR' iv_message = lx_error->get_text( ) ).
        RETURN.
    ENDTRY.

    " Success response - note that execution is async
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"status":"started","report":"{ lv_report }","task":"{ lv_task }","mode":"async_rfc"{ lv_c }|.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.


  METHOD handle_read_source.
    " Native READ REPORT — reads source for any program/include, including
    " SUBC=I (enhancement plug-in source). Bypasses RPY_PROGRAM_READ which
    " raises CANCELLED in RFC contexts on classic ECC.
    DATA(lv_progname_str) = extract_param( iv_params = is_message-params iv_name = 'program' ).
    IF lv_progname_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter program is required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_progname_str TO UPPER CASE.

    DATA lv_progname TYPE progname.
    lv_progname = lv_progname_str.

    DATA lt_source TYPE TABLE OF string.

    TRY.
        READ REPORT lv_progname INTO lt_source.
      CATCH cx_root INTO DATA(lx_err).
        rs_response = build_error( iv_id = is_message-id iv_code = 'READ_FAILED' iv_message = lx_err->get_text( ) ).
        RETURN.
    ENDTRY.

    IF sy-subrc <> 0.
      rs_response = build_error( iv_id = is_message-id iv_code = 'NOT_FOUND' iv_message = |Program { lv_progname } not found| ).
      RETURN.
    ENDIF.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"program":"{ escape_json( lv_progname_str ) }","source":[|.

    DATA lv_first TYPE abap_bool VALUE abap_true.
    LOOP AT lt_source INTO DATA(lv_line).
      IF lv_first = abap_false.
        lv_json = |{ lv_json },|.
      ENDIF.
      lv_json = |{ lv_json }"{ escape_json( lv_line ) }"|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }]{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_write_enho_source.
    TYPES:
      BEGIN OF ty_source_element,
        id     TYPE int4,
        source TYPE rswsourcet,
      END OF ty_source_element,
      tt_source_elements TYPE STANDARD TABLE OF ty_source_element WITH KEY id.

    DATA(lv_enhname_str) = extract_param( iv_params = is_message-params iv_name = 'enhancement' ).
    DATA(lv_source_b64) = extract_param( iv_params = is_message-params iv_name = 'source_base64' ).
    DATA(lv_transport_str) = extract_param( iv_params = is_message-params iv_name = 'transport' ).
    IF lv_enhname_str IS INITIAL OR lv_source_b64 IS INITIAL.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'MISSING_PARAM'
        iv_message = 'Parameters enhancement and source_base64 are required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_enhname_str TO UPPER CASE.
    TRANSLATE lv_transport_str TO UPPER CASE.
    IF strlen( lv_enhname_str ) > 30
       OR lv_enhname_str CN 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_/$'.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'INVALID_ENHANCEMENT'
        iv_message = 'Enhancement name contains unsupported characters' ).
      RETURN.
    ENDIF.
    IF lv_source_b64 CN 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'INVALID_SOURCE'
        iv_message = 'source_base64 contains unsupported characters' ).
      RETURN.
    ENDIF.
    IF lv_transport_str IS NOT INITIAL.
      IF strlen( lv_transport_str ) > 20
         OR lv_transport_str CN 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_/'.
        rs_response = build_error(
          iv_id = is_message-id iv_code = 'INVALID_TRANSPORT'
          iv_message = 'Transport contains unsupported characters' ).
        RETURN.
      ENDIF.
    ENDIF.

    " Validate package and CTS ownership before starting the isolated worker.
    " The worker executes this same check again immediately before locking.
    DATA lv_enhname TYPE enhname.
    DATA lv_trkorr TYPE trkorr.
    DATA lv_devclass TYPE devclass.
    DATA lv_owner_task TYPE trkorr.
    DATA lv_owner_request TYPE trkorr.
    DATA lv_korrflag TYPE tdevc-korrflag.
    lv_enhname = lv_enhname_str.
    lv_trkorr = lv_transport_str.
    SELECT SINGLE devclass FROM tadir INTO lv_devclass
      WHERE pgmid = 'R3TR' AND object = 'ENHO' AND obj_name = lv_enhname.
    IF sy-subrc <> 0.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'ENHANCEMENT_NOT_FOUND'
        iv_message = |Enhancement { lv_enhname } is not registered in TADIR| ).
      RETURN.
    ENDIF.
    SELECT SINGLE korrflag FROM tdevc INTO lv_korrflag
      WHERE devclass = lv_devclass.
    IF lv_korrflag = 'X' AND lv_trkorr IS INITIAL.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'TRANSPORT_REQUIRED'
        iv_message = |Package { lv_devclass } requires an explicit transport| ).
      RETURN.
    ENDIF.
    SELECT SINGLE trkorr FROM e071 INTO lv_owner_task
      WHERE pgmid = 'R3TR' AND object = 'ENHO' AND obj_name = lv_enhname
        AND lockflag = 'X'.
    IF lv_owner_task IS NOT INITIAL.
      SELECT SINGLE strkorr FROM e070 INTO lv_owner_request
        WHERE trkorr = lv_owner_task.
      IF lv_owner_request IS INITIAL.
        lv_owner_request = lv_owner_task.
      ENDIF.
      IF lv_trkorr IS NOT INITIAL AND lv_trkorr <> lv_owner_task
         AND lv_trkorr <> lv_owner_request.
        rs_response = build_error(
          iv_id = is_message-id iv_code = 'TRANSPORT_MISMATCH'
          iv_message = |Requested { lv_trkorr } but CTS owns the ENHO in { lv_owner_request }| ).
        RETURN.
      ENDIF.
    ENDIF.

    " Enhancement persistence performs commits that are not legal in an APC
    " work process. Schedule the exact same handler in an isolated RFC work
    " process; the Go client confirms completion through active read-back.
    IF gv_enho_worker_mode = abap_false.
      DATA lt_program TYPE TABLE OF progtab.
      DATA lv_program_line TYPE string.
      APPEND 'REPORT z$$$xrfc.' TO lt_program.
      APPEND 'DATA lv_b64 TYPE string.' TO lt_program.
      DATA lv_remaining TYPE string.
      DATA lv_chunk TYPE string.
      lv_remaining = lv_source_b64.
      WHILE lv_remaining IS NOT INITIAL.
        IF strlen( lv_remaining ) > 48.
          lv_chunk = lv_remaining(48).
          lv_remaining = lv_remaining+48.
        ELSE.
          lv_chunk = lv_remaining.
          CLEAR lv_remaining.
        ENDIF.
        lv_program_line = |lv_b64 = lv_b64 && '{ lv_chunk }'.|.
        APPEND lv_program_line TO lt_program.
      ENDWHILE.
      APPEND 'zcl_vsp_rfc_service=>write_enho_worker(' TO lt_program.
      lv_program_line = |iv_enhname = '{ lv_enhname_str }'|.
      APPEND lv_program_line TO lt_program.
      APPEND 'iv_source_b64 = lv_b64' TO lt_program.
      lv_program_line = |iv_transport = '{ lv_transport_str }' ).|.
      APPEND lv_program_line TO lt_program.

      DATA(lv_task) = |VSP_ENHO_{ sy-uzeit }|.
      CALL FUNCTION 'RFC_ABAP_INSTALL_AND_RUN'
        STARTING NEW TASK lv_task
        EXPORTING mode = 'F'
        TABLES program = lt_program
        EXCEPTIONS communication_failure = 1 system_failure = 2
                   resource_failure = 3 OTHERS = 4.
      IF sy-subrc <> 0.
        rs_response = build_error(
          iv_id = is_message-id iv_code = 'ENHO_WORKER_START_FAILED'
          iv_message = |Could not start ENHO worker: { sy-subrc }| ).
        RETURN.
      ENDIF.

      DATA(lv_o_scheduled) = '{'.
      DATA(lv_c_scheduled) = '}'.
      DATA(lv_scheduled_json) = |{ lv_o_scheduled }"status":"scheduled","task":"{ lv_task }"{ lv_c_scheduled }|.
      rs_response = VALUE #( id = is_message-id success = abap_true data = lv_scheduled_json ).
      RETURN.
    ENDIF.

    DATA lv_source_text TYPE string.
    TRY.
        DATA(lv_source_x) = cl_http_utility=>decode_x_base64( lv_source_b64 ).
        lv_source_text = cl_abap_codepage=>convert_from( source = lv_source_x codepage = `UTF-8` ).
      CATCH cx_root INTO DATA(lx_decode).
        rs_response = build_error(
          iv_id = is_message-id iv_code = 'INVALID_SOURCE'
          iv_message = lx_decode->get_text( ) ).
        RETURN.
    ENDTRY.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf
      IN lv_source_text WITH cl_abap_char_utilities=>newline.

    DATA lt_lines TYPE rswsourcet.
    DATA lt_elements TYPE tt_source_elements.
    DATA ls_element TYPE ty_source_element.
    DATA lv_inside TYPE abap_bool.
    DATA lv_line TYPE string.
    DATA lv_header TYPE string.
    DATA lv_id_text TYPE string.
    DATA lv_header_rest TYPE string.
    SPLIT lv_source_text AT cl_abap_char_utilities=>newline INTO TABLE lt_lines.
    LOOP AT lt_lines INTO lv_line.
      lv_header = lv_line.
      CONDENSE lv_header.
      IF lv_inside = abap_false.
        IF lv_header IS INITIAL.
          CONTINUE.
        ENDIF.
        IF lv_header NP 'ENHANCEMENT *.'.
          rs_response = build_error(
            iv_id = is_message-id iv_code = 'INVALID_SOURCE'
            iv_message = 'Expected ENHANCEMENT <id>. wrapper' ).
          RETURN.
        ENDIF.
        lv_id_text = lv_header+12.
        CONDENSE lv_id_text.
        SPLIT lv_id_text AT space INTO lv_id_text lv_header_rest.
        REPLACE ALL OCCURRENCES OF '.' IN lv_id_text WITH ''.
        IF lv_id_text IS INITIAL OR lv_id_text CN '0123456789'.
          rs_response = build_error(
            iv_id = is_message-id iv_code = 'INVALID_SOURCE'
            iv_message = 'Enhancement element id is missing' ).
          RETURN.
        ENDIF.
        CLEAR ls_element.
        ls_element-id = lv_id_text.
        lv_inside = abap_true.
      ELSEIF lv_header = 'ENDENHANCEMENT.'.
        INSERT ls_element INTO TABLE lt_elements.
        IF sy-subrc <> 0.
          rs_response = build_error(
            iv_id = is_message-id iv_code = 'INVALID_SOURCE'
            iv_message = 'Duplicate enhancement element id' ).
          RETURN.
        ENDIF.
        CLEAR ls_element.
        lv_inside = abap_false.
      ELSE.
        APPEND lv_line TO ls_element-source.
      ENDIF.
    ENDLOOP.
    IF lv_inside = abap_true OR lt_elements IS INITIAL.
      rs_response = build_error(
        iv_id = is_message-id iv_code = 'INVALID_SOURCE'
        iv_message = 'Enhancement source has an unclosed or empty wrapper' ).
      RETURN.
    ENDIF.

    DATA lo_tool TYPE REF TO if_enh_tool.
    DATA lo_hook TYPE REF TO cl_enh_tool_hook_impl.
    DATA lt_current TYPE enh_hook_impl_it.
    DATA ls_current TYPE enh_hook_impl.
    DATA lv_version TYPE r3state VALUE 'A'.
    TRY.
        cl_enh_factory=>get_enhancement(
          EXPORTING enhancement_id = lv_enhname lock = abap_true
                    suppress_lang_dialog = abap_true run_dark = abap_true
                    adt_resource = abap_true
          RECEIVING enhancement = lo_tool ).
        lo_hook ?= lo_tool.

        DATA(lv_locked_trkorr) = lo_tool->if_enh_object~get_trkorr( ).
        DATA lv_locked_request TYPE trkorr.
        IF lv_locked_trkorr IS NOT INITIAL.
          SELECT SINGLE strkorr FROM e070 INTO lv_locked_request
            WHERE trkorr = lv_locked_trkorr.
          IF lv_locked_request IS INITIAL.
            lv_locked_request = lv_locked_trkorr.
          ENDIF.
        ENDIF.
        IF lv_trkorr IS NOT INITIAL AND lv_locked_trkorr IS NOT INITIAL
           AND lv_trkorr <> lv_locked_trkorr AND lv_trkorr <> lv_locked_request.
          lo_tool->if_enh_object~unlock( ).
          rs_response = build_error(
            iv_id = is_message-id iv_code = 'TRANSPORT_MISMATCH'
            iv_message = |Requested { lv_trkorr } but SAP locked the ENHO in { lv_locked_trkorr }| ).
          RETURN.
        ENDIF.

        IF lo_tool->if_enh_object~has_inactive_version( ) = abap_true.
          lv_version = 'I'.
        ENDIF.
        lt_current = lo_hook->get_hook_impls( lv_version ).

        LOOP AT lt_elements INTO ls_element.
          READ TABLE lt_current INTO ls_current WITH KEY id = ls_element-id.
          IF sy-subrc <> 0.
            lo_tool->if_enh_object~unlock( ).
            rs_response = build_error(
              iv_id = is_message-id iv_code = 'ELEMENT_NOT_FOUND'
              iv_message = |Enhancement element { ls_element-id } does not exist| ).
            RETURN.
          ENDIF.
          lo_hook->modify_hook_impl(
            extid = ls_current-extid overwrite = ls_current-overwrite
            method = ls_current-method enhmode = ls_current-enhmode
            full_name = ls_current-full_name source = ls_element-source
            spot = ls_current-spotname parent_full_name = ls_current-parent_full_name ).
        ENDLOOP.

        lo_tool->if_enh_object~save(
          EXPORTING run_dark = abap_true
          CHANGING devclass = lv_devclass trkorr = lv_trkorr ).
        lo_tool->if_enh_object~activate(
          EXPORTING run_dark = abap_true
          CHANGING devclass = lv_devclass trkorr = lv_trkorr ).
        lo_tool->if_enh_object~unlock( ).

      CATCH cx_root INTO DATA(lx_write).
        IF lo_tool IS BOUND.
          TRY.
              IF lo_tool->if_enh_object~is_locked( ) = abap_true.
                lo_tool->if_enh_object~unlock( ).
              ENDIF.
            CATCH cx_root.
          ENDTRY.
        ENDIF.
        rs_response = build_error(
          iv_id = is_message-id iv_code = 'ENHO_WRITE_ERROR'
          iv_message = lx_write->get_text( ) ).
        RETURN.
    ENDTRY.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA(lv_count) = lines( lt_elements ).
    DATA(lv_json) = |{ lv_o }"enhancement":"{ lv_enhname }","elements":{ lv_count }{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD write_enho_worker.
    DATA ls_message TYPE zif_vsp_service=>ty_message.
    DATA ls_response TYPE zif_vsp_service=>ty_response.
    DATA lo_service TYPE REF TO zcl_vsp_rfc_service.
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    ls_message-id = 'worker'.
    ls_message-action = 'writeEnhancementSource'.
    ls_message-params = |{ lv_o }"enhancement":"{ iv_enhname }","source_base64":"{ iv_source_b64 }","transport":"{ iv_transport }"{ lv_c }|.

    gv_enho_worker_mode = abap_true.
    CREATE OBJECT lo_service.
    ls_response = lo_service->handle_write_enho_source( ls_message ).
    gv_enho_worker_mode = abap_false.
    IF ls_response-success = abap_false.
      MESSAGE ls_response-error TYPE 'X'.
    ENDIF.
  ENDMETHOD.

ENDCLASS.
