PRN "sleeping for 100 sec"
SLP 100
PRN "Woke up ready to send"
SND 0 "Hello World" 
PRN "Waiting to receive from 0" 
RCV 1 $v2
PRN "Received " $v2
