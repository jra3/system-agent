// Copyright (c) 2024 Antimetal
package test

import "fmt"

func   BadlyFormatted(  x    int,y string)string{
fmt.Println("This has bad formatting")
    return    y
}

type  UnformattedStruct   struct{
Name   string
    Value int}

func(u*UnformattedStruct)GetName()string{
return u.Name}
