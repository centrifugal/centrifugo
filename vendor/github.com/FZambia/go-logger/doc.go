/*
Package logger is a level-based logger for Go applications.

Example usage:
  package main

  import (
    "github.com/FZambia/go-logger"
  )

  func main() {
    name := "Alexander"
    logger.INFO.Printf("Hello %s", name)
    logger.ERROR.Println("Error")
  }
Output:
  [I]: 2016/09/09 19:41:13 Hello Alexander
  [E]: 2016/09/09 19:41:13 Error
*/
package logger
