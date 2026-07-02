// Trimmed excerpt of Newtonsoft.Json JsonTextReader.cs, pinned commit
// 4f73e74372445108d2c1bda37b36e6f5e43402e0. Copyright (c) 2007 James
// Newton-King, MIT License. Used only as a gotreesitter parse-recovery
// regression fixture for issue #136 (see JamesNK/Newtonsoft.Json).
namespace Newtonsoft.Json
{
    internal enum ReadType
    {
        Read,
        ReadAsInt32,
        ReadAsInt64,
        ReadAsBytes,
        ReadAsString,
        ReadAsDecimal,
        ReadAsDateTime,
#if HAVE_DATE_TIME_OFFSET
        ReadAsDateTimeOffset,
#endif
        ReadAsDouble,
        ReadAsBoolean
    }

    /// <summary>
    /// Represents a reader that provides fast, non-cached, forward-only access to JSON text data.
    /// </summary>
    public partial class JsonTextReader : JsonReader, IJsonLineInfo
    {
        private const char UnicodeReplacementChar = '\uFFFD';
#if HAVE_BIG_INTEGER
        private const int MaximumJavascriptIntegerCharacterLength = 380;
#endif
#if DEBUG
        internal int LargeBufferLength { get; set; } = int.MaxValue / 2;
#else
        private const int LargeBufferLength = int.MaxValue / 2;
#endif

        private readonly TextReader _reader;
        private char[]? _chars;
        private int _charsUsed;
        private int _charPos;
        private int _lineStartPos;
        private int _lineNumber;
        private bool _isEndOfFile;
        private StringBuffer _stringBuffer;
        private StringReference _stringReference;
        private IArrayPool<char>? _arrayPool;

        /// <summary>
        /// Initializes a new instance of the <see cref="JsonTextReader"/> class with the specified <see cref="TextReader"/>.
        /// </summary>
        /// <param name="reader">The <see cref="TextReader"/> containing the JSON data to read.</param>
        public JsonTextReader(TextReader reader)
        {
            if (reader == null)
            {
                throw new ArgumentNullException(nameof(reader));
            }

            _reader = reader;
            _lineNumber = 1;

#if HAVE_ASYNC
            _safeAsync = GetType() == typeof(JsonTextReader);
#endif
        }

#if DEBUG
        internal char[]? CharBuffer
        {
            get => _chars;
            set => _chars = value;
        }

        internal int CharPos => _charPos;
#endif

        /// <summary>
        /// Gets or sets the reader's property name table.
        /// </summary>
        public JsonNameTable? PropertyNameTable { get; set; }

        /// <summary>
        /// Gets or sets the reader's character buffer pool.
        /// </summary>
        public IArrayPool<char>? ArrayPool
        {
            get => _arrayPool;
            set
            {
                if (value == null)
                {
                    throw new ArgumentNullException(nameof(value));
                }

                _arrayPool = value;
            }
        }

        private void EnsureBufferNotEmpty()
        {
            if (_stringBuffer.IsEmpty)
            {
                _stringBuffer = new StringBuffer(_arrayPool, 1024);
            }
        }

        private void SetNewLine(bool hasNextChar)
        {
            MiscellaneousUtils.Assert(_chars != null);

            if (hasNextChar && _chars[_charPos] == StringUtils.LineFeed)
            {
                _charPos++;
            }

            OnNewLine(_charPos);
        }

        private void OnNewLine(int pos)
        {
            _lineNumber++;
            _lineStartPos = pos;
        }

        private void ParseString(char quote, ReadType readType)
        {
            _charPos++;

            ShiftBufferIfNeeded();
            ReadStringIntoBuffer(quote);
            ParseReadString(quote, readType);
        }

        private void ParseReadString(char quote, ReadType readType)
        { 
            SetPostValueState(true);

            switch (readType)
            {
                case ReadType.ReadAsBytes:
                    Guid g;
                    byte[] data;
                    if (_stringReference.Length == 0)
                    {
                        data = CollectionUtils.ArrayEmpty<byte>();
                    }
                    else if (_stringReference.Length == 36 && ConvertUtils.TryConvertGuid(_stringReference.ToString(), out g))
                    {
                        data = g.ToByteArray();
                    }
                    else
                    {
                        data = Convert.FromBase64CharArray(_stringReference.Chars, _stringReference.StartIndex, _stringReference.Length);
                    }

                    SetToken(JsonToken.Bytes, data, false);
                    break;
                case ReadType.ReadAsString:
                    string text = _stringReference.ToString();

                    SetToken(JsonToken.String, text, false);
                    _quoteChar = quote;
                    break;
                case ReadType.ReadAsInt32:
                case ReadType.ReadAsDecimal:
                case ReadType.ReadAsBoolean:
                    // caller will convert result
                    break;
                default:
                    if (_dateParseHandling != DateParseHandling.None)
                    {
                        DateParseHandling dateParseHandling;
                        if (readType == ReadType.ReadAsDateTime)
                        {
                            dateParseHandling = DateParseHandling.DateTime;
                        }
#if HAVE_DATE_TIME_OFFSET
                        else if (readType == ReadType.ReadAsDateTimeOffset)
                        {
                            dateParseHandling = DateParseHandling.DateTimeOffset;
                        }
#endif
                        else
                        {
                            dateParseHandling = _dateParseHandling;
                        }

                        if (dateParseHandling == DateParseHandling.DateTime)
                        {
                            if (DateTimeUtils.TryParseDateTime(_stringReference, DateTimeZoneHandling, DateFormatString, Culture, out DateTime dt))
                            {
                                SetToken(JsonToken.Date, dt, false);
                                return;
                            }
                        }
#if HAVE_DATE_TIME_OFFSET
                        else
                        {
                            if (DateTimeUtils.TryParseDateTimeOffset(_stringReference, DateFormatString, Culture, out DateTimeOffset dt))
                            {
                                SetToken(JsonToken.Date, dt, false);
                                return;
                            }
                        }
#endif
                    }

                    SetToken(JsonToken.String, _stringReference.ToString(), false);
                    _quoteChar = quote;
                    break;
            }
        }

        private static void BlockCopyChars(char[] src, int srcOffset, char[] dst, int dstOffset, int count)
        {
            const int charByteCount = 2;

            Buffer.BlockCopy(src, srcOffset * charByteCount, dst, dstOffset * charByteCount, count * charByteCount);
        }

        private void ShiftBufferIfNeeded()
        {
            MiscellaneousUtils.Assert(_chars != null);

            // once in the last 10% of the buffer, or buffer is already very large then
            // shift the remaining content to the start to avoid unnecessarily increasing
            // the buffer size when reading numbers/strings
            int length = _chars.Length;
            if (length - _charPos <= length * 0.1 || length >= LargeBufferLength)
            {
                int count = _charsUsed - _charPos;
                if (count > 0)
                {
                    BlockCopyChars(_chars, _charPos, _chars, 0, count);
                }

                _lineStartPos -= _charPos;
                _charPos = 0;
                _charsUsed = count;
                _chars[_charsUsed] = '\0';
            }
        }

        private int ReadData(bool append)
        {
            return ReadData(append, 0);
        }

        private void PrepareBufferForReadData(bool append, int charsRequired)
        {
            MiscellaneousUtils.Assert(_chars != null);

            // char buffer is full
            if (_charsUsed + charsRequired >= _chars.Length - 1)
            {
                if (append)
                {
                    int doubledArrayLength = _chars.Length * 2;

                    // copy to new array either double the size of the current or big enough to fit required content
                    int newArrayLength = Math.Max(
                        doubledArrayLength < 0 ? int.MaxValue : doubledArrayLength, // handle overflow
                        _charsUsed + charsRequired + 1);

                    // increase the size of the buffer
                    char[] dst = BufferUtils.RentBuffer(_arrayPool, newArrayLength);

                    BlockCopyChars(_chars, 0, dst, 0, _chars.Length);

                    BufferUtils.ReturnBuffer(_arrayPool, _chars);

                    _chars = dst;
                }
                else
                {
                    int remainingCharCount = _charsUsed - _charPos;

                    if (remainingCharCount + charsRequired + 1 >= _chars.Length)
                    {
                        // the remaining count plus the required is bigger than the current buffer size
                        char[] dst = BufferUtils.RentBuffer(_arrayPool, remainingCharCount + charsRequired + 1);

                        if (remainingCharCount > 0)
                        {
                            BlockCopyChars(_chars, _charPos, dst, 0, remainingCharCount);
                        }

                        BufferUtils.ReturnBuffer(_arrayPool, _chars);

                        _chars = dst;
                    }
                    else
                    {
                        // copy any remaining data to the beginning of the buffer if needed and reset positions
                        if (remainingCharCount > 0)
                        {
                            BlockCopyChars(_chars, _charPos, _chars, 0, remainingCharCount);
                        }
                    }

                    _lineStartPos -= _charPos;
                    _charPos = 0;
                    _charsUsed = remainingCharCount;
                }
            }
        }

        private int ReadData(bool append, int charsRequired)
        {
            if (_isEndOfFile)
            {
                return 0;
            }

            PrepareBufferForReadData(append, charsRequired);
            MiscellaneousUtils.Assert(_chars != null);

            int attemptCharReadCount = _chars.Length - _charsUsed - 1;

            int charsRead = _reader.Read(_chars, _charsUsed, attemptCharReadCount);

            _charsUsed += charsRead;

            if (charsRead == 0)
            {
                _isEndOfFile = true;
            }

            _chars[_charsUsed] = '\0';
            return charsRead;
        }

        private bool EnsureChars(int relativePosition, bool append)
        {
            if (_charPos + relativePosition >= _charsUsed)
            {
                return ReadChars(relativePosition, append);
            }

            return true;
        }

        private bool ReadChars(int relativePosition, bool append)
        {
            if (_isEndOfFile)
            {
                return false;
            }

            int charsRequired = _charPos + relativePosition - _charsUsed + 1;

            int totalCharsRead = 0;

            // it is possible that the TextReader doesn't return all data at once
            // repeat read until the required text is returned or the reader is out of content
            do
            {
                int charsRead = ReadData(append, charsRequired - totalCharsRead);

                // no more content
                if (charsRead == 0)
                {
                    break;
                }

                totalCharsRead += charsRead;
            } while (totalCharsRead < charsRequired);

            if (totalCharsRead < charsRequired)
            {
                return false;
            }
            return true;
        }
    }
}
