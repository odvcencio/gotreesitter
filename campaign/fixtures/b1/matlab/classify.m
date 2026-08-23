function out = classify(status)
%CLASSIFY Map a raw status to its census bucket.
switch lower(status)
    case 'pass'
        out = 'PASS';
    case {'fallback', 'skip'}
        out = 'DECLINE';
    otherwise
        out = 'UNKNOWN';
end
end
