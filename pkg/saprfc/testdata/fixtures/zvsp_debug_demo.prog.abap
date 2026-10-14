REPORT zvsp_debug_demo.

TYPES: BEGIN OF ty_row,
         id   TYPE i,
         name TYPE string,
       END OF ty_row.

FORM double USING iv_in TYPE i CHANGING cv_out TYPE i.
  cv_out = iv_in * 2.
ENDFORM.

START-OF-SELECTION.
  DATA: lv_counter TYPE i VALUE 7,
        lv_doubled TYPE i,
        ls_row     TYPE ty_row,
        lt_rows    TYPE STANDARD TABLE OF ty_row WITH EMPTY KEY.

  ls_row-id   = 1.
  ls_row-name = 'first'.
  APPEND ls_row TO lt_rows.

  ls_row-id   = 2.
  ls_row-name = 'second'.
  APPEND ls_row TO lt_rows.

  PERFORM double USING lv_counter CHANGING lv_doubled.

  WRITE: / 'counter', lv_counter, 'doubled', lv_doubled, 'rows', lines( lt_rows ).
