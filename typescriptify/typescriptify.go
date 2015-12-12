package typescriptify

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
)

type TypeScriptify struct {
	Prefix         string
	Suffix         string
	Indent         string
	FromJSONMethod bool

	golangTypes []reflect.Type
	types       map[reflect.Kind]string

	// throwaway, used when converting
	alreadyConverted map[reflect.Type]bool
}

func New() *TypeScriptify {
	result := new(TypeScriptify)
	result.Indent = "\t"

	types := make(map[reflect.Kind]string)

	types[reflect.Bool] = "boolean"

	types[reflect.Int] = "number"
	types[reflect.Int8] = "number"
	types[reflect.Int16] = "number"
	types[reflect.Int32] = "number"
	types[reflect.Int64] = "number"
	types[reflect.Uint] = "number"
	types[reflect.Uint8] = "number"
	types[reflect.Uint16] = "number"
	types[reflect.Uint32] = "number"
	types[reflect.Uint64] = "number"
	types[reflect.Float32] = "number"
	types[reflect.Float64] = "number"

	types[reflect.String] = "string"

	result.types = types

	return result
}

func (this *TypeScriptify) Add(obj interface{}) {
	this.AddType(reflect.TypeOf(obj))
}

func (this *TypeScriptify) AddType(typeOf reflect.Type) {
	this.golangTypes = append(this.golangTypes, typeOf)
}

func (this *TypeScriptify) Convert(customCode map[string]string) (string, error) {
	this.alreadyConverted = make(map[reflect.Type]bool)

	result := ""
	for _, typeof := range this.golangTypes {
		typeScriptCode, err := this.convertType(typeof, customCode)
		if err != nil {
			return "", err
		}
		result += "\n" + strings.Trim(typeScriptCode, " "+this.Indent+"\r\n")
	}
	return result, nil
}

func loadCustomCode(fileName string) (map[string]string, error) {
	result := make(map[string]string)
	f, err := os.Open(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	defer f.Close()

	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return result, err
	}

	var currentName string
	var currentValue string
	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "//[") && strings.HasSuffix(trimmedLine, ":]") {
			currentName = strings.Replace(strings.Replace(trimmedLine, "//[", "", -1), ":]", "", -1)
			currentValue = ""
		} else if trimmedLine == "//[end]" {
			result[currentName] = strings.TrimRight(currentValue, " \t\r\n")
			currentName = ""
			currentValue = ""
		} else if len(currentName) > 0 {
			currentValue += line + "\n"
		}
	}

	return result, nil
}

func (this TypeScriptify) ConvertToFile(fileName string) error {
	customCode, err := loadCustomCode(fileName)
	if err != nil {
		return err
	}

	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	converted, err := this.Convert(customCode)
	if err != nil {
		return err
	}

	f.WriteString("/* Do not change, this code is generated from Golang structs */\n\n")
	f.WriteString(converted)
	if err != nil {
		return err
	}

	return nil
}

func (this *TypeScriptify) convertType(typeOf reflect.Type, customCode map[string]string) (string, error) {
	if _, found := this.alreadyConverted[typeOf]; found { // Already converted
		return "", nil
	}

	entityName := fmt.Sprintf("%s%s%s", this.Prefix, this.Suffix, typeOf.Name())
	result := fmt.Sprintf("class %s {\n", entityName)
	builder := TypeScriptClassBuilder{
		types:  this.types,
		indent: this.Indent,
	}

	for i := 0; i < typeOf.NumField(); i++ {
		val := typeOf.Field(i)
		//fmt.Println("kind=", val.Type.Kind().String())
		jsonTag := val.Tag.Get("json")
		jsonFieldName := ""
		if len(jsonTag) > 0 {
			jsonTagParts := strings.Split(jsonTag, ",")
			if len(jsonTagParts) > 0 {
				jsonFieldName = strings.Trim(jsonTagParts[0], this.Indent)
			}
		}
		if len(jsonFieldName) > 0 && jsonFieldName != "-" {
			var err error
			if val.Type.Kind() == reflect.Struct { // Struct:
				typeScriptChunk, err := this.convertType(val.Type, customCode)
				if err != nil {
					return "", err
				}
				result = typeScriptChunk + "\n" + result
				builder.AddStructField(jsonFieldName, val.Type.Name())
			} else if val.Type.Kind() == reflect.Slice { // Slice:
				if val.Type.Elem().Kind() == reflect.Struct { // Slice of structs:
					typeScriptChunk, err := this.convertType(val.Type.Elem(), customCode)
					if err != nil {
						return "", err
					}
					result = typeScriptChunk + "\n" + result
					builder.AddArrayOfStructsField(jsonFieldName, val.Type.Elem().Name())
				} else { // Slice of simple fields:
					err = builder.AddSimpleArrayField(jsonFieldName, val.Type.Elem().Name(), val.Type.Elem().Kind())
				}
			} else { // Simple field:
				err = builder.AddSimpleField(jsonFieldName, val.Type.Name(), val.Type.Kind())
			}
			if err != nil {
				return "", err
			}
		}
	}

	result += builder.fields
	if this.FromJSONMethod {
		result += fmt.Sprintf("%sstatic createFrom(source: any) {\n", this.Indent)
		result += fmt.Sprintf("%s%svar result = new %s();\n", this.Indent, this.Indent, entityName)
		result += builder.createFromMethodBody
		result += fmt.Sprintf("%s%sreturn result;\n", this.Indent, this.Indent)
		result += fmt.Sprintf("%s}\n", this.Indent)
	}

	if customCode != nil {
		code := customCode[entityName]
		result += this.Indent + "//[" + entityName + ":]\n" + code + "\n\n" + this.Indent + "//[end]\n"
	}

	result += "}"

	this.alreadyConverted[typeOf] = true

	return result, nil
}

type TypeScriptClassBuilder struct {
	types                map[reflect.Kind]string
	indent               string
	fields               string
	createFromMethodBody string
}

func (this *TypeScriptClassBuilder) AddSimpleArrayField(fieldName, fieldType string, kind reflect.Kind) error {
	if typeScriptType, ok := this.types[kind]; ok {
		if len(fieldName) > 0 {
			this.fields += fmt.Sprintf("%s%s: %s[];\n", this.indent, fieldName, typeScriptType)
			this.createFromMethodBody += fmt.Sprintf("%s%sresult.%s = source[\"%s\"];\n", this.indent, this.indent, fieldName, fieldName)
			return nil
		}
	}
	return errors.New(fmt.Sprintf("Cannot find type for %s (%s/%s)", kind.String(), fieldName, fieldType))
}

func (this *TypeScriptClassBuilder) AddSimpleField(fieldName, fieldType string, kind reflect.Kind) error {
	if typeScriptType, ok := this.types[kind]; ok {
		if len(fieldName) > 0 {
			this.fields += fmt.Sprintf("%s%s: %s;\n", this.indent, fieldName, typeScriptType)
			this.createFromMethodBody += fmt.Sprintf("%s%sresult.%s = source[\"%s\"];\n", this.indent, this.indent, fieldName, fieldName)
			return nil
		}
	}
	return errors.New("Cannot find type for " + fieldType)
}

func (this *TypeScriptClassBuilder) AddStructField(fieldName, fieldType string) {
	this.fields += fmt.Sprintf("%s%s: %s;\n", this.indent, fieldName, fieldType)
	this.createFromMethodBody += fmt.Sprintf("%s%sresult.%s = %s.source(json[\"%s\"]);\n", this.indent, this.indent, fieldName, fieldType, fieldName)
}

func (this *TypeScriptClassBuilder) AddArrayOfStructsField(fieldName, fieldType string) {
	this.fields += fmt.Sprintf("%s%s: %s[];\n", this.indent, fieldName, fieldType)
	this.createFromMethodBody += fmt.Sprintf("%s%sif (source[\"%s\"]) {\n", this.indent, this.indent, fieldName)
	this.createFromMethodBody += fmt.Sprintf("%s%s%sresult.%s = source[\"%s\"].map(function(element) { return %s.fromJSON(element); });\n", this.indent, this.indent, this.indent, fieldName, fieldName, fieldType)
	this.createFromMethodBody += fmt.Sprintf("%s%s}\n", this.indent, this.indent)
}