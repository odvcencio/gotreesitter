:- module(census, [classify/2]).

classify(pass, pass).
classify(fallback, decline).
classify(skip, decline).
classify(_, unknown).
