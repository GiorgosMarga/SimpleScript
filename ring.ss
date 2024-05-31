#SIMPLESCRIPT     

     SET $myid $argv1 
     SET $nxtid $argv2
     SET $prvid $argv3
     SET $cnt $argv4
     SET $time $argv5
     
     SET $val 0

     BEQ $myid 0 #L2

#L1  
     PRN $myid " waiting to receive from " $prvid
     RCV $myid $val
     PRN $myid " received " $val " from " $prvid
     BEQ $cnt 0 #L3
   
#L2  
     PRN $myid " sleeping for " $time
     SLP $time
     
     ADD $val $val 1
     PRN $myid " sending " $val " to " $nxtid
     SND $nxtid $val
     SUB $cnt $cnt 1
     
     BEQ $myid 0 #L1
     BGT $cnt 0 #L1
     
#L3 
     PRN "finished"
     RET
     
     