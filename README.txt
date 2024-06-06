1)
multiply.txt 
------------
* Takes as arguments two integers <a> and <b>
* Calculates the product via addition
* Assumes <a> is >= 0, else terminates

Example:
run multiply.ss 5 15

2) 
sender.txt
----------
* Takes as arguments an integer <tid>, an integer <cnt>, a string <msg> and an integer <time> 
* Waits <time> seconds and sends to thread <tid> the message <msg>
* This is repeated <cnt> times  

receiver.txt
------------
* Takes as arguments an integer <tid> and an integer <cnt>
* Receives from thread <tid> a text message and prints it 
* This is repeated <cnt> times 

Example for 3 iterations:
run sender.ss 1 3 hello 5 || receiver.ss 1 3

3)
ring.txt
--------
* Takes as arguments three integers <myid>, <nxtid>, <prvid>, an integer <cnt> and an integer <time>
* Sends along a ring, according to <nxtid> and <prvid>, a token with an initial value of 1
* The token value is incremented after each hop
* Forwarding is done after a delay of <time> seconds
* The token makes <cnt> circles around the ring   

Example for a ring of 3 threads and 5 iterations:
run ring.ss 0 1 2 5 10 || ring.ss 1 2 0 5 10 || ring.ss 2 0 1 5 10
migrate 0 0 127.0.0.0 :3002
migrate 0 1 127.0.0.0 :3003

4) 
    run multiply_m.ss 10 20 || multiply_m.ss 10 20
    migrate 0 0 127.0.0.0 :3002

5)
    run testRecvM.ss || testSndM.ss
    migrate 0 0 127.0.0.0 :3002
