
typedef string bigString<>;

program TEST_PROG {
  version TEST_V1 {
    void test_null(void) = 0;
    int test_inc(int) = 1;
    int test_add(int, int) = 2;
    bigString test_string(bigString) = 3;
  } = 1;
} = 0x233ec478;
