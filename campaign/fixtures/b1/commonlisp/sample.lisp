(defpackage #:census
  (:use #:cl)
  (:export #:classify))
(in-package #:census)

(defun classify (status)
  "Map a raw status keyword to its census bucket."
  (case status
    ((pass) :pass)
    ((fallback skip) :decline)
    (t :unknown)))
