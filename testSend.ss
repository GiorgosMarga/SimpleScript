PRN "Sleeping for 10 secs"
SET $val1 10
SLP 100
PRN "Woke up. Ready to send..."
SND 0 "Hello world"
PRN "SENT"