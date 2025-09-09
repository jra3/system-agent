#include <linux/bpf.h>

int   unformatted_func(   int x   )   {
    if(x>100){return   -1;}
return   x*2;}

struct   test_struct{
    int  field1;
int    field2;};
