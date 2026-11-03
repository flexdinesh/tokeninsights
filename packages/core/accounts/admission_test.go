package accounts

import "testing"

func TestAdmissionDoesNotStarveAnotherUser(t *testing.T) {
	s := &Service{admission: newAdmission(), login: newLoginAdmission()}
	a := Principal{UserID: "a"}
	b := Principal{UserID: "b"}
	release, err := s.Admit(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Admit(a); err == nil {
		t.Fatal("parallel same-user admission")
	}
	other, err := s.Admit(b)
	if err != nil {
		t.Fatal("one user blocks another", err)
	}
	other()
	release()
	release()
	for range BatchBurst - 1 {
		release, err := s.Admit(a)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := s.Admit(a); err == nil {
		t.Fatal("unbounded user burst")
	}
	for range LoginRequestsPerMinute {
		if err := s.AdmitLogin("127.0.0.1:1234"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AdmitLogin("127.0.0.1:5678"); err == nil {
		t.Fatal("login source port bypass")
	}
	if err := s.AdmitLogin("127.0.0.2:1234"); err != nil {
		t.Fatal("other login peer blocked", err)
	}
}
