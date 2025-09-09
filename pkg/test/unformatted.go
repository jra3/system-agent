package test

import "fmt"

func   UnformattedFunction(  x    int,y string)string{
fmt.Println("Bad formatting")
    return    y
}

type  BadStruct   struct{
Field1   string
    Field2 int
    Field3    bool}

func(b*BadStruct)Method()string{
return b.Field1}
