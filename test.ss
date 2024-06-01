SET $val1 0
SET $val2 100

#Label1:
    ADD $val1 $val1 1
    BEQ $val1 $val2 #Label2
    BRA #Label1

#Label2:
    SLP 100
    PRN "Label2"