package generator

import (
	"encoding/json"
	"fmt"
	"github.com/golang-collections/collections/queue"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var que *queue.Queue
var configQue *queue.Queue
var singleQue *queue.Queue

func GenerateModels(ConfigFilePath string, GenerateModelPath string) {
	que = queue.New()
	configQue = queue.New()
	singleQue = queue.New()
	jsonFile, err := os.Open(ConfigFilePath)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)
	var config interface{}
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Panic(err)
	}
	processMap(config.(map[string]interface{}))
	f, err := os.Create(GenerateModelPath)
	if err != nil {
		log.Panic(err.Error())
	}
	m := config.(map[string]interface{})
	//panic: interface conversion: interface {} is map[string]interface {}, not main.Hello
	//Cannot use assets to convert an interface to a struct
	f.WriteString("package config\n")
	configTypeGenerator(m, f)

	for que.Len() != 0 {
		name := que.Dequeue().(string)
		name = strings.Title(strings.ToLower(name))
		f.Write([]byte("\n"))
		configQue.Enqueue(name)
		f.WriteString("type " + name + " struct{")
		f.Write([]byte("\n"))
		modelGenerator(que.Dequeue().(map[string]interface{}), f)
		f.WriteString("}")
	}
	f.Write([]byte("\n"))
	f.WriteString("type Config struct{")
	for singleQue.Len() != 0 {
		f.Write([]byte("\n"))
		_name := singleQue.Dequeue().(string)
		_type := singleQue.Dequeue().(string)
		f.WriteString("\t" + _name + " " + _type)
	}
	for configQue.Len() != 0 {
		f.Write([]byte("\n"))
		n := configQue.Dequeue().(string)
		f.WriteString("\t" + n + " " + n)
	}
	f.Write([]byte("\n"))
	f.WriteString("}")
}
func configTypeGenerator(m map[string]interface{}, file *os.File) {
	for index, value := range m {
		t := getType(value)
		if t == "interface" {
			que.Enqueue(index)
			que.Enqueue(value)
		} else {
			singleQue.Enqueue(index)
			singleQue.Enqueue(t)
		}
	}
}
func modelGenerator(m map[string]interface{}, file *os.File) {
	for index, value := range m {
		t := getType(value)
		if t == "interface" {
			que.Enqueue(index)
			que.Enqueue(value)
		} else {
			file.WriteString("\t" + strings.Title(strings.ToLower(index)) + " " + t)
			file.Write([]byte("\n"))
		}
	}
}
func getType(val interface{}) string {
	if _, ok := val.(int); ok {
		return "int"
	} else if _, ok := val.(float64); ok {
		return "float64"
	} else if _, ok := val.(string); ok {
		return "string"
	} else if _, ok := val.(interface{}); ok {
		return "interface"
	} else {
		return "halt"
	}
}

// toInt 如果 f 是整数，则返回 int 类型的值，否则返回原始 float64
func toInt(f float64) interface{} {
	// 检查 f 是否为整数
	if float64(int(f)) == f {
		return int(f)
	}
	return f
}

// processMap 递归处理 map 中的所有值
func processMap(m map[string]interface{}) {
	for k, v := range m {
		switch v := v.(type) {
		case map[string]interface{}:
			processMap(v) // 递归处理嵌套 map
		case float64:
			m[k] = toInt(v) // 处理数字，确保整数被转换为 int
		}
	}
}
