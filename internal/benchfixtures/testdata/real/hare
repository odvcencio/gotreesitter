// SPDX-License-Identifier: MPL-2.0
// (c) Hare authors <https://harelang.org>

// #define __ADDR_BND_PKEY_PAD (_Alignof(void *) < sizeof(short) ? sizeof(short) : _Alignof(void *))
def __ADDR_BND_PKEY_PAD: size = 8;

// Returns the new PID to the parent, void to the child, or errno if something
// goes wrong.
export fn clone(
	stack: nullable *opaque,
	flags: int,
	parent_tid: nullable *int,
	child_tid: nullable *int,
	tls: u64,
) (int | void | errno) = {
	match (wrap_return(syscall5(SYS_clone,
		flags: u64, stack: uintptr: u64,
		parent_tid: uintptr: u64,
		tls, child_tid: uintptr: u64))) {
	case let u: u64 =>
		switch (u) {
		case 0 =>
			return;
		case =>
			return u: int;
		};
	case let err: errno =>
		return err;
	};
};

export def O_DIRECTORY: int = 0o200000;
export def O_DIRECT: int = 0o40000;

export type cmsghdr = struct {
	cmsg_len: socklen_t,
	_: int, // padding
	cmsg_level: int,
	cmsg_type: int,
};

export def EDEADLOCK: int = EDEADLK;

export type epoll_event = struct {
	events: u32,
	data: epoll_data,
};

export type cpu_set = struct {
	__bits: [16]u64,
};

export type ucontext_t = struct {
	uc_flags: u64,
	uc_link: *ucontext_t,
	uc_stack: stack_t,
	uc_sigmask: sigset,
	_: [1024 / 8 - size(sigset)]u8, // padding
	uc_mcontext: sigcontext,
};

export type sigcontext = struct {
	sc_regs: user_regs,
	sc_fpregs: fp_state,
};

export type user_regs = struct {
	x0: u64, // XXX: Why is this here?
	pc: u64,
	ra: u64,
	sp: u64,
	gp: u64,
	tp: u64,
	t0: u64,
	t1: u64,
	t2: u64,
	s0: u64,
	s1: u64,
	a0: u64,
	a1: u64,
	a2: u64,
	a3: u64,
	a4: u64,
	a5: u64,
	a6: u64,
	a7: u64,
	s2: u64,
	s3: u64,
	s4: u64,
	s5: u64,
	s6: u64,
	s7: u64,
	s8: u64,
	s9: u64,
	s10: u64,
	s11: u64,
	t3: u64,
	t4: u64,
	t5: u64,
	t6: u64,
};

export type fp_state = union {
	f: f_ext_state,
	d: d_ext_state,
	q: q_ext_state,
};

export type f_ext_state = struct {
	f: [32]u32,
	fcsr: u32,
};

export type d_ext_state = struct {
	f: [32]u64,
	fcsr: u32,
};

export type q_ext_state = struct {
	f: [64]u64,
	fcsr: u32,
	_: [3]u32, // reserved
};
