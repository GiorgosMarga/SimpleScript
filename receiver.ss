#SIMPLESCRIPT   
     SET $tid $argv1
     SET $cnt $argv2

#L1
     BLT $cnt 1 #L2 
     PRN "receiving from " $tid
     RCV $tid $msg
     PRN "received " $msg
     SUB $cnt $cnt 1
     BRA #L1
     
#L2 
     RET
     
     