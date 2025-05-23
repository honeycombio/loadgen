# grpcsink

This is just a little tool you can point a grpc sender at, and it will count the number of unique traces and the number of unique spans sent in a session. It emits the count and stops when you close it with ^C.