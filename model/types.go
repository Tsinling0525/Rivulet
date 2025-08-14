package model

import "time"

type ID string

type Port string

const (
	PortMain Port = "main"
)

type Edge struct {
	FromNode ID
	FromPort Port
	ToNode   ID
	ToPort   Port
}

type Node struct {
	ID          ID
	Type        string
	Name        string
	Concurrency int           // 0 = default
	Timeout     time.Duration // 0 = none
	Config      map[string]any
	Credentials string // reference key
}

type Workflow struct {
	ID    ID
	Name  string
	Nodes []Node
	Edges []Edge
}

type Item = map[string]any

type Items = []Item
