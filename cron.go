package chrono

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

var (
	months = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	days   = []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"}
)

type cronField string

const (
	cronFieldSecond     = "SECOND"
	cronFieldMinute     = "MINUTE"
	cronFieldHour       = "HOUR"
	cronFieldDayOfMonth = "DAY_OF_MONTH"
	cronFieldMonth      = "MONTH"
	cronFieldDayOfWeek  = "DAY_OF_WEEK"
)

type fieldType struct {
	Field    cronField
	MinValue int
	MaxValue int
}

var (
	second     = fieldType{cronFieldSecond, 0, 59}
	minute     = fieldType{cronFieldMinute, 0, 59}
	hour       = fieldType{cronFieldHour, 0, 23}
	dayOfMonth = fieldType{cronFieldDayOfMonth, 1, 31}
	month      = fieldType{cronFieldMonth, 1, 12}
	dayOfWeek  = fieldType{cronFieldDayOfWeek, 0, 6}
)

var cronFieldTypes = []fieldType{
	second,
	minute,
	hour,
	dayOfMonth,
	month,
	dayOfWeek,
}

type valueRange struct {
	MinValue int
	MaxValue int
}

func newValueRange(min int, max int) valueRange {
	return valueRange{
		MinValue: min,
		MaxValue: max,
	}
}

type cronFieldBits struct {
	Typ  fieldType
	Bits uint64
}

func newFieldBits(typ fieldType) *cronFieldBits {
	return &cronFieldBits{
		Typ: typ,
	}
}

const maxAttempts = 366
const mask = 0xFFFFFFFFFFFFFFFF

type CronExpression struct {
	fields []*cronFieldBits
}

func newCronExpression() *CronExpression {
	return &CronExpression{
		make([]*cronFieldBits, 0),
	}
}

func (expression *CronExpression) NextTime(t time.Time) time.Time {

	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)

	for i := 0; i < maxAttempts; i++ {
		result := expression.next(t)

		if result.IsZero() || result.Equal(t) {
			return result
		}

		t = result
	}

	return time.Time{}
}

func (expression *CronExpression) next(t time.Time) time.Time {
	for _, field := range expression.fields {

		temp := t
		current := getTimeValue(temp, field.Typ.Field)

		next := setNextBit(field.Bits, current)

		if next == -1 {
			amount := field.Typ.MaxValue - current + 1
			temp = addTime(temp, field.Typ.Field, amount)
			next = setNextBit(field.Bits, 0)
		}

		if next == current {
			return t
		} else {
			count := 0
			current := getTimeValue(temp, field.Typ.Field)
			for ; current != next && count < maxAttempts; count++ {

			}

			if count >= maxAttempts {
				return time.Time{}
			}

		}

	}

	return time.Time{}
}

func ParseCronExpression(expression string) (*CronExpression, error) {
	if len(expression) == 0 {
		return nil, errors.New("cron expression must not be empty")
	}

	fields := strings.Fields(expression)

	if len(fields) != 6 {
		return nil, fmt.Errorf("cron expression must consist of 6 fields : found %d in \"%s\"", len(fields), expression)
	}

	cronExpression := newCronExpression()

	for index, cronFieldType := range cronFieldTypes {
		value, err := parseField(fields[index], cronFieldType)

		if err != nil {
			return nil, err
		}

		cronExpression.fields = append(cronExpression.fields, value)
	}

	return cronExpression, nil
}

func parseField(value string, fieldType fieldType) (*cronFieldBits, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("value must not be empty")
	}

	if fieldType.Field == cronFieldMonth {
		value = replaceOrdinals(value, months)
	} else if fieldType.Field == cronFieldDayOfWeek {
		value = replaceOrdinals(value, days)
	}

	cronFieldBits := newFieldBits(fieldType)

	fields := strings.Split(value, ",")

	for _, field := range fields {
		slashPos := strings.Index(field, "/")

		step := -1
		var valueRange valueRange

		if slashPos != -1 {
			rangeStr := field[0:slashPos]

			var err error
			valueRange, err = parseRange(rangeStr, fieldType)

			if err != nil {
				return nil, err
			}

			if strings.Index(rangeStr, "-") == -1 {
				valueRange = newValueRange(valueRange.MinValue, fieldType.MaxValue)
			}

			stepStr := field[slashPos+1:]

			step, err = strconv.Atoi(stepStr)

			if err != nil {
				panic(err)
			}

			if step <= 0 {
				panic("step must be 1 or higher")
			}

		} else {
			var err error
			valueRange, err = parseRange(field, fieldType)

			if err != nil {
				return nil, err
			}
		}

		if step > 1 {
			for index := valueRange.MinValue; index <= valueRange.MaxValue; index += step {
				cronFieldBits.Bits |= 1 << index
			}
			continue
		}

		if valueRange.MinValue == valueRange.MaxValue {
			cronFieldBits.Bits |= 1 << valueRange.MinValue
		} else {
			cronFieldBits.Bits = ^(math.MaxUint64 << (valueRange.MaxValue + 1)) & (math.MaxUint64 << valueRange.MinValue)
		}
	}

	return cronFieldBits, nil
}

func parseRange(value string, fieldType fieldType) (valueRange, error) {
	if value == "*" {
		return newValueRange(fieldType.MinValue, fieldType.MaxValue), nil
	} else {
		hyphenPos := strings.Index(value, "-")

		if hyphenPos == -1 {
			result, err := checkValidValue(value, fieldType)

			if err != nil {
				return valueRange{}, err
			}

			return newValueRange(result, result), nil
		} else {
			maxStr := value[hyphenPos+1:]
			minStr := value[0:hyphenPos]

			min, err := checkValidValue(minStr, fieldType)

			if err != nil {
				return valueRange{}, err
			}
			var max int
			max, err = checkValidValue(maxStr, fieldType)

			if err != nil {
				return valueRange{}, err
			}

			if fieldType.Field == cronFieldDayOfWeek && min == 7 {
				min = 0
			}

			return newValueRange(min, max), nil
		}
	}
}

func replaceOrdinals(value string, list []string) string {
	value = strings.ToUpper(value)

	for index := 0; index < len(list); index++ {
		replacement := strconv.Itoa(index + 1)
		value = strings.ReplaceAll(value, list[index], replacement)
	}

	return value
}

func checkValidValue(value string, fieldType fieldType) (int, error) {
	result, err := strconv.Atoi(value)

	if err != nil {
		return 0, fmt.Errorf("the value in field %s must be number : %s", fieldType.Field, value)
	}

	if fieldType.Field == cronFieldDayOfWeek && result == 0 {
		return result, nil
	}

	if result >= fieldType.MinValue && result <= fieldType.MaxValue {
		return result, nil
	}

	return 0, fmt.Errorf("the value in field %s must be between %d and %d", fieldType.Field, fieldType.MinValue, fieldType.MaxValue)
}

func getTimeValue(t time.Time, field cronField) int {

	switch field {
	case cronFieldSecond:
		return t.Second()
	case cronFieldMinute:
		return t.Minute()
	case cronFieldHour:
		return t.Hour()
	case cronFieldDayOfMonth:
		return t.Day()
	case cronFieldMonth:
		return int(t.Month())
	case cronFieldDayOfWeek:
		return int(t.Weekday())
	}

	panic("unreachable code")
}

func addTime(t time.Time, field cronField, value int) time.Time {
	switch field {
	case cronFieldSecond:
		return t.Add(time.Duration(value) * time.Second)
	case cronFieldMinute:
		return t.Add(time.Duration(value) * time.Minute)
	case cronFieldHour:
		return t.Add(time.Duration(value) * time.Hour)
	case cronFieldDayOfMonth:
		return t.AddDate(0, 0, value)
	case cronFieldMonth:
		return t.AddDate(0, value, 0)
	case cronFieldDayOfWeek:
		return t.AddDate(0, 0, value)
	}

	panic("unreachable code")
}

func setNextBit(bitsValue uint64, index int) int {
	result := bitsValue & (mask << index)

	if result != 0 {
		return bits.TrailingZeros64(result)
	}

	return -1
}
