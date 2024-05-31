#SIMPLESCRIPT     

     PRN "this is " $argv0
     
     SET $a $argv1
     SET $b $argv2
     SET $res 0

     BLT $a 0 #L3

     PRN "computing " $a " * " $b     
     
#L1  
     BEQ $a 0 #L2
     ADD $res $res $b
     PRN "intermediate result is " $res
     SUB $a $a 1
     BRA #L1

#L2  
     PRN "final result is " $res
     RET

#L3  
     PRN "first argument is <0 " $a 
     RET    
     