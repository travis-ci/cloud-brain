package database

// Ensure that MemoryDatabase implements the DB interface
var _ DB = &MemoryDatabase{}
