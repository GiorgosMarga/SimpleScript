#SIMPLESCRIPT     
     SET $tid $argv1
     SET $cnt $argv2
     SET $msg $argv3
     SET $time $argv4

#L1  
     BLT $cnt 1 #L2
     PRN "sleeping for " $time " seconds"
     SLP $time
     PRN "sending " $msg " to " $tid
     SND $tid $msg
     SUB $cnt $cnt 1
     
     BRA #L1

#L2
     RET
     
     