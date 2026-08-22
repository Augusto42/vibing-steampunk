REPORT zvsp_debug_bigtable.

TYPES: BEGIN OF ty_row,
         id   TYPE i,
         name TYPE string,
       END OF ty_row.

START-OF-SELECTION.
  DATA: ls_row  TYPE ty_row,
        lt_rows TYPE STANDARD TABLE OF ty_row WITH EMPTY KEY.

  DO 250 TIMES.
    ls_row-id   = sy-index.
    ls_row-name = |row{ sy-index }|.
    APPEND ls_row TO lt_rows.
  ENDDO.

  WRITE: / 'rows', lines( lt_rows ).
