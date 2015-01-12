# Redis-view

`redis-view` is a `tree` like tool help you expore data structures in your redis server

## Installation
```
go get github.com/dreamersdw/redis-view
go install github.com/dreamersdw/redis-view
```

## Usage
```
Usage:
	redis-view [--url=URL] [--sep=SEP] [--only-keys] [--nowrap] [PATTERN...]
	redis-view --version
	redis-view --help

Example:
	redis-view 'tasks:*' 'metrics:*'
```

## Screenshot
![redis-view](https://raw.githubusercontent.com/dreamersdw/redis-view/master/screenshot/redis-view.png)

