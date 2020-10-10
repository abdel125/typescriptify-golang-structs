package typescriptify

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/tkrajina/go-reflector/reflector"
)

const (
	tsTransformTag      = "ts_transform"
	tsType              = "ts_type"
	tsConvertValuesFunc = `convertValues(a: any, classs: any, asMap: boolean = false): any {
	if (!a) {
		return a;
	}
	if (a.slice) {
		return (a as any[]).map(elem => this.convertValues(elem, classs));
	} else if ("object" === typeof a) {
		if (asMap) {
			for (const key of Object.keys(a)) {
				a[key] = new classs(a[key]);
			}
			return a;
		}
		return new classs(a);
	}
	return a;
}`
)

type FieldOptions struct {
	TSType      string
	TSTransform string
}

type StructType struct {
	Type         reflect.Type
	FieldOptions map[reflect.Type]FieldOptions
}
type EnumType struct {
	Type reflect.Type
}

type enumElement struct {
	value interface{}
	name  string
}

type TypeScriptify struct {
	Prefix            string
	Suffix            string
	Indent            string
	CreateFromMethod  bool
	CreateConstructor bool
	BackupDir         string // If empty no backup
	DontExport        bool
	CreateInterface   bool
	customImports     []string

	structTypes []StructType
	enumTypes   []EnumType
	enums       map[reflect.Type][]enumElement
	kinds       map[reflect.Kind]string

	// throwaway, used when converting
	alreadyConverted map[reflect.Type]bool
}

func New() *TypeScriptify {
	result := new(TypeScriptify)
	result.Indent = "\t"
	result.BackupDir = "."

	types := make(map[reflect.Kind]string)

	types[reflect.Bool] = "boolean"
	types[reflect.Interface] = "any"

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

	result.kinds = types

	result.Indent = "    "
	result.CreateFromMethod = true
	result.CreateConstructor = true

	return result
}

func deepFields(typeOf reflect.Type) []reflect.StructField {
	fields := make([]reflect.StructField, 0)

	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}

	if typeOf.Kind() != reflect.Struct {
		return fields
	}

	for i := 0; i < typeOf.NumField(); i++ {
		f := typeOf.Field(i)

		kind := f.Type.Kind()
		if f.Anonymous && kind == reflect.Struct {
			//fmt.Println(v.Interface())
			fields = append(fields, deepFields(f.Type)...)
		} else if f.Anonymous && kind == reflect.Ptr && f.Type.Elem().Kind() == reflect.Struct {
			//fmt.Println(v.Interface())
			fields = append(fields, deepFields(f.Type.Elem())...)
		} else {
			fields = append(fields, f)
		}
	}

	return fields
}

func (t *TypeScriptify) WithCreateFromMethod(b bool) *TypeScriptify {
	t.CreateFromMethod = b
	return t
}

func (t *TypeScriptify) WithConstructor(b bool) *TypeScriptify {
	t.CreateConstructor = b
	return t
}

func (t *TypeScriptify) WithIndent(i string) *TypeScriptify {
	t.Indent = i
	return t
}

func (t *TypeScriptify) WithBackupDir(b string) *TypeScriptify {
	t.BackupDir = b
	return t
}

func (t *TypeScriptify) WithPrefix(p string) *TypeScriptify {
	t.Prefix = p
	return t
}

func (t *TypeScriptify) WithSuffix(s string) *TypeScriptify {
	t.Suffix = s
	return t
}

func (t *TypeScriptify) Add(obj interface{}) *TypeScriptify {
	switch ty := obj.(type) {
	case StructType:
		t.structTypes = append(t.structTypes, ty)
		break
	case *StructType:
		t.structTypes = append(t.structTypes, *ty)
		break
	default:
		t.AddType(reflect.TypeOf(obj))
		break
	}
	return t
}

func (t *TypeScriptify) AddType(typeOf reflect.Type) *TypeScriptify {
	t.structTypes = append(t.structTypes, StructType{Type: typeOf})
	return t
}

func (t *typeScriptClassBuilder) AddMapField(fieldName string, field reflect.StructField) {
	keyType := field.Type.Key()
	valueType := field.Type.Elem()
	valueTypeName := valueType.Name()
	if name, ok := t.types[valueType.Kind()]; ok {
		valueTypeName = name
	}
	if valueType.Kind() == reflect.Array || valueType.Kind() == reflect.Slice {
		valueTypeName = valueType.Elem().Name() + "[]"
	}
	if valueType.Kind() == reflect.Ptr {
		valueTypeName = valueType.Elem().Name()
	}
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")

	t.fields = append(t.fields, fmt.Sprintf("%s%s: {[key: %s]: %s};", t.indent, fieldName, t.prefix+keyType.Name()+t.suffix, valueTypeName))
	if valueType.Kind() == reflect.Struct {
		t.constructorBody = append(t.constructorBody, fmt.Sprintf("%s%sthis.%s = this.convertValues(source[\"%s\"], %s, true);", t.indent, t.indent, strippedFieldName, strippedFieldName, t.prefix+valueTypeName+t.suffix))
	} else {
		t.constructorBody = append(t.constructorBody, fmt.Sprintf("%s%sthis.%s = source[\"%s\"];", t.indent, t.indent, strippedFieldName, strippedFieldName))
	}
}

func (t *TypeScriptify) AddEnum(values interface{}) *TypeScriptify {
	if t.enums == nil {
		t.enums = map[reflect.Type][]enumElement{}
	}
	items := reflect.ValueOf(values)
	if items.Kind() != reflect.Slice {
		panic(fmt.Sprintf("Values for %T isn't a slice", values))
	}

	var elements []enumElement
	for i := 0; i < items.Len(); i++ {
		item := items.Index(i)

		var el enumElement
		if item.Kind() == reflect.Struct {
			r := reflector.New(item.Interface())
			val, err := r.Field("Value").Get()
			if err != nil {
				panic(fmt.Sprint("missing Type field in ", item.Type().String()))
			}
			name, err := r.Field("TSName").Get()
			if err != nil {
				panic(fmt.Sprint("missing TSName field in ", item.Type().String()))
			}
			el.value = val
			el.name = name.(string)
		} else {
			el.value = item.Interface()
			if tsNamer, is := item.Interface().(TSNamer); is {
				el.name = tsNamer.TSName()
			} else {
				panic(fmt.Sprint(item.Type().String(), " has no TSName method"))
			}
		}

		elements = append(elements, el)
	}
	ty := reflect.TypeOf(elements[0].value)
	t.enums[ty] = elements
	t.enumTypes = append(t.enumTypes, EnumType{Type: ty})

	return t
}

// AddEnumValues is deprecated, use `AddEnum()`
func (t *TypeScriptify) AddEnumValues(typeOf reflect.Type, values interface{}) *TypeScriptify {
	t.AddEnum(values)
	return t
}

func (t *TypeScriptify) Convert(customCode map[string]string) (string, error) {
	t.alreadyConverted = make(map[reflect.Type]bool)

	result := ""
	if len(t.customImports) > 0 {
		// Put the custom imports, i.e.: `import Decimal from 'decimal.js'`
		for _, cimport := range t.customImports {
			result += cimport + "\n"
		}
	}

	for _, enumTyp := range t.enumTypes {
		elements := t.enums[enumTyp.Type]
		typeScriptCode, err := t.convertEnum(enumTyp.Type, elements)
		if err != nil {
			return "", err
		}
		result += "\n" + strings.Trim(typeScriptCode, " "+t.Indent+"\r\n")
	}

	for _, strctTyp := range t.structTypes {
		typeScriptCode, err := t.convertType(strctTyp.Type, customCode)
		if err != nil {
			return "", err
		}
		result += "\n" + strings.Trim(typeScriptCode, " "+t.Indent+"\r\n")
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

func (t TypeScriptify) backup(fileName string) error {
	fileIn, err := os.Open(fileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// No neet to backup, just return:
		return nil
	}
	defer fileIn.Close()

	bytes, err := ioutil.ReadAll(fileIn)
	if err != nil {
		return err
	}

	_, backupFn := path.Split(fmt.Sprintf("%s-%s.backup", fileName, time.Now().Format("2006-01-02T15_04_05.99")))
	if t.BackupDir != "" {
		backupFn = path.Join(t.BackupDir, backupFn)
	}

	return ioutil.WriteFile(backupFn, bytes, os.FileMode(0700))
}

func (t TypeScriptify) ConvertToFile(fileName string) error {
	if len(t.BackupDir) > 0 {
		err := t.backup(fileName)
		if err != nil {
			return err
		}
	}

	customCode, err := loadCustomCode(fileName)
	if err != nil {
		return err
	}

	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	converted, err := t.Convert(customCode)
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

type TSNamer interface {
	TSName() string
}

func (t *TypeScriptify) convertEnum(typeOf reflect.Type, elements []enumElement) (string, error) {
	if _, found := t.alreadyConverted[typeOf]; found { // Already converted
		return "", nil
	}
	t.alreadyConverted[typeOf] = true

	entityName := t.Prefix + typeOf.Name() + t.Suffix
	result := "enum " + entityName + " {\n"

	for _, val := range elements {
		result += fmt.Sprintf("%s%s = %#v,\n", t.Indent, val.name, val.value)
	}

	result += "}"

	if !t.DontExport {
		result = "export " + result
	}

	return result, nil
}

func (t *TypeScriptify) getFieldOptions(structType reflect.Type, field reflect.StructField) FieldOptions {
	/*
		Here find the struct in t.structTypes and get a custom FieldOptions or use the one defined with tags:
	*/
	return FieldOptions{TSTransform: field.Tag.Get(tsTransformTag), TSType: field.Tag.Get(tsType)}
}

func (t *TypeScriptify) convertType(typeOf reflect.Type, customCode map[string]string) (string, error) {
	if _, found := t.alreadyConverted[typeOf]; found { // Already converted
		return "", nil
	}
	t.alreadyConverted[typeOf] = true

	entityName := t.Prefix + typeOf.Name() + t.Suffix
	result := ""
	if t.CreateInterface {
		result += fmt.Sprintf("interface %s {\n", entityName)
	} else {
		result += fmt.Sprintf("class %s {\n", entityName)
	}
	if !t.DontExport {
		result = "export " + result
	}
	builder := typeScriptClassBuilder{
		types:  t.kinds,
		indent: t.Indent,
		prefix: t.Prefix,
		suffix: t.Suffix,
	}

	fields := deepFields(typeOf)
	for _, field := range fields {
		isPtr := field.Type.Kind() == reflect.Ptr
		if isPtr {
			field.Type = field.Type.Elem()
		}
		jsonTag := field.Tag.Get("json")
		jsonFieldName := ""
		if len(jsonTag) > 0 {
			jsonTagParts := strings.Split(jsonTag, ",")
			if len(jsonTagParts) > 0 {
				jsonFieldName = strings.Trim(jsonTagParts[0], t.Indent)
			}
			hasOmitEmpty := false
			for _, t := range jsonTagParts {
				if t == "" {
					break
				}
				if t == "omitempty" {
					hasOmitEmpty = true
					break
				}
			}
			if isPtr || hasOmitEmpty {
				jsonFieldName = fmt.Sprintf("%s?", jsonFieldName)
			}
		}
		if len(jsonFieldName) > 0 && jsonFieldName != "-" {
			var err error
			fldOpts := t.getFieldOptions(typeOf, field)
			if fldOpts.TSTransform != "" {
				err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
			} else if _, isEnum := t.enums[field.Type]; isEnum {
				builder.AddEnumField(jsonFieldName, field)
			} else if fldOpts.TSType != "" { // Struct:
				err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
			} else if field.Type.Kind() == reflect.Struct { // Struct:
				typeScriptChunk, err := t.convertType(field.Type, customCode)
				if err != nil {
					return "", err
				}
				if typeScriptChunk != "" {
					result = typeScriptChunk + "\n" + result
				}
				builder.AddStructField(jsonFieldName, field)
			} else if field.Type.Kind() == reflect.Map {
				// Also convert map key types if needed
				var keyTypeToConvert reflect.Type
				switch field.Type.Key().Kind() {
				case reflect.Struct:
					keyTypeToConvert = field.Type.Key()
				case reflect.Ptr:
					keyTypeToConvert = field.Type.Key().Elem()
				}
				if keyTypeToConvert != nil {
					typeScriptChunk, err := t.convertType(keyTypeToConvert, customCode)
					if err != nil {
						return "", err
					}
					if typeScriptChunk != "" {
						result = typeScriptChunk + "\n" + result
					}
				}
				// Also convert map value types if needed
				var valueTypeToConvert reflect.Type
				switch field.Type.Elem().Kind() {
				case reflect.Struct:
					valueTypeToConvert = field.Type.Elem()
				case reflect.Ptr:
					valueTypeToConvert = field.Type.Elem().Elem()
				}
				if valueTypeToConvert != nil {
					typeScriptChunk, err := t.convertType(valueTypeToConvert, customCode)
					if err != nil {
						return "", err
					}
					if typeScriptChunk != "" {
						result = typeScriptChunk + "\n" + result
					}
				}

				builder.AddMapField(jsonFieldName, field)
			} else if field.Type.Kind() == reflect.Slice { // Slice:
				if field.Type.Elem().Kind() == reflect.Ptr { //extract ptr type
					field.Type = field.Type.Elem()
				}

				arrayDepth := 1
				for field.Type.Elem().Kind() == reflect.Slice { // Slice of slices:
					field.Type = field.Type.Elem()
					arrayDepth++
				}

				if field.Type.Elem().Kind() == reflect.Struct { // Slice of structs:
					typeScriptChunk, err := t.convertType(field.Type.Elem(), customCode)
					if err != nil {
						return "", err
					}
					if typeScriptChunk != "" {
						result = typeScriptChunk + "\n" + result
					}
					builder.AddArrayOfStructsField(jsonFieldName, field, arrayDepth)
				} else { // Slice of simple fields:
					err = builder.AddSimpleArrayField(jsonFieldName, field, arrayDepth, fldOpts)
				}
			} else { // Simple field:
				err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
			}
			if err != nil {
				return "", err
			}
		}
	}

	if t.CreateFromMethod {
		fmt.Fprintln(os.Stderr, "FromMethod METHOD IS DEPRECATED AND WILL BE REMOVED!!!!!!")
		t.CreateConstructor = true
	}

	result += strings.Join(builder.fields, "\n") + "\n"
	if !t.CreateInterface {
		constructorBody := strings.Join(builder.constructorBody, "\n")
		needsConvertValue := strings.Contains(constructorBody, "this.convertValues")
		if t.CreateFromMethod {
			result += fmt.Sprintf("\n%sstatic createFrom(source: any = {}) {\n", t.Indent)
			result += fmt.Sprintf("%s%sreturn new %s(source);\n", t.Indent, t.Indent, entityName)
			result += fmt.Sprintf("%s}\n", t.Indent)
		}
		if t.CreateConstructor {
			result += fmt.Sprintf("\n%sconstructor(source: any = {}) {\n", t.Indent)
			result += t.Indent + t.Indent + "if ('string' === typeof source) source = JSON.parse(source);\n"
			result += constructorBody + "\n"
			result += fmt.Sprintf("%s}\n", t.Indent)
		}
		if needsConvertValue && (t.CreateConstructor || t.CreateFromMethod) {
			result += "\n" + indentLines(tsConvertValuesFunc, 1) + "\n"
		}
	}

	if customCode != nil {
		code := customCode[entityName]
		if len(code) != 0 {
			result += t.Indent + "//[" + entityName + ":]\n" + code + "\n\n" + t.Indent + "//[end]\n"
		}
	}

	result += "}"

	return result, nil
}

func (t *TypeScriptify) AddImport(i string) {
	for _, cimport := range t.customImports {
		if cimport == i {
			return
		}
	}

	t.customImports = append(t.customImports, i)
}

type typeScriptClassBuilder struct {
	types                map[reflect.Kind]string
	indent               string
	fields               []string
	createFromMethodBody []string
	constructorBody      []string
	prefix, suffix       string
}

func (t *typeScriptClassBuilder) AddSimpleArrayField(fieldName string, field reflect.StructField, arrayDepth int, opts FieldOptions) error {
	fieldType, kind := field.Type.Elem().Name(), field.Type.Elem().Kind()
	typeScriptType := t.types[kind]

	if len(fieldName) > 0 {
		strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
		if len(opts.TSType) > 0 {
			t.addField(fieldName, opts.TSType)
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
			return nil
		} else if len(typeScriptType) > 0 {
			t.addField(fieldName, fmt.Sprint(typeScriptType, strings.Repeat("[]", arrayDepth)))
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
			return nil
		}
	}

	return errors.New(fmt.Sprintf("cannot find type for %s (%s/%s)", kind.String(), fieldName, fieldType))
}

func (t *typeScriptClassBuilder) AddSimpleField(fieldName string, field reflect.StructField, opts FieldOptions) error {
	fieldType, kind := field.Type.Name(), field.Type.Kind()

	typeScriptType := t.types[kind]
	if len(opts.TSType) > 0 {
		typeScriptType = opts.TSType
	}

	if len(typeScriptType) > 0 && len(fieldName) > 0 {
		strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
		t.addField(fieldName, typeScriptType)
		if opts.TSTransform == "" {
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
		} else {
			val := fmt.Sprintf(`source["%s"]`, strippedFieldName)
			expression := strings.Replace(opts.TSTransform, "__VALUE__", val, -1)
			t.addInitializerFieldLine(strippedFieldName, expression)
		}
		return nil
	}

	return errors.New("Cannot find type for " + fieldType + ", fideld: " + fieldName)
}

func (t *typeScriptClassBuilder) AddEnumField(fieldName string, field reflect.StructField) {
	fieldType := field.Type.Name()
	t.addField(fieldName, t.prefix+fieldType+t.suffix)
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
}

func (t *typeScriptClassBuilder) AddStructField(fieldName string, field reflect.StructField) {
	fieldType := field.Type.Name()
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addField(fieldName, t.prefix+fieldType+t.suffix)
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("this.convertValues(source[\"%s\"], %s)", strippedFieldName, t.prefix+fieldType+t.suffix))
}

func (t *typeScriptClassBuilder) AddArrayOfStructsField(fieldName string, field reflect.StructField, arrayDepth int) {
	fieldType := field.Type.Elem().Name()
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addField(fieldName, fmt.Sprint(t.prefix+fieldType+t.suffix, strings.Repeat("[]", arrayDepth)))
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("this.convertValues(source[\"%s\"], %s)", strippedFieldName, t.prefix+fieldType+t.suffix))
}

func (t *typeScriptClassBuilder) addInitializerFieldLine(fld, initializer string) {
	t.createFromMethodBody = append(t.createFromMethodBody, fmt.Sprint(t.indent, t.indent, "result.", fld, " = ", initializer, ";"))
	t.constructorBody = append(t.constructorBody, fmt.Sprint(t.indent, t.indent, "this.", fld, " = ", initializer, ";"))
}

func (t *typeScriptClassBuilder) addField(fld, fldType string) {
	t.fields = append(t.fields, fmt.Sprint(t.indent, fld, ": ", fldType, ";"))
}
